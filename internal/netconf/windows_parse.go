package netconf

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// Parsers for Windows PowerShell JSON, `netsh wlan` output and the proxy
// registry blob. Build-tag free so their tests run everywhere.

// psInterface mirrors the PowerShell object emitted for one adapter; field
// shapes are stable because PS builds them explicitly.
type psInterface struct {
	Name         string `json:"name"`
	Label        string `json:"label"`
	Type         string `json:"type"`
	HardwareAddr string `json:"hardware_addr"`
	MTU          int    `json:"mtu"`
	Up           bool   `json:"up"`
	IPs          []struct {
		Address string `json:"address"`
		Prefix  int    `json:"prefix"`
		Family  string `json:"family"`
		Dynamic bool   `json:"dynamic"`
	} `json:"ips"`
	DHCP    bool     `json:"dhcp"`
	Gateway string   `json:"gateway"`
	DNS     []string `json:"dns"`
}

// parsePSInterfaces parses the PowerShell adapter JSON into Interface
// values, converting prefix lengths to netmasks for IPv4.
func parsePSInterfaces(out string) ([]Interface, error) {
	out = strings.TrimSpace(out)
	if out == "" {
		return nil, nil
	}
	var list []psInterface
	dec := json.NewDecoder(strings.NewReader(out))
	// PS 5.1 may emit a bare object when the adapter list has one entry.
	if strings.HasPrefix(out, "{") {
		var single psInterface
		if err := dec.Decode(&single); err != nil {
			return nil, newError(CodeCommandFailed, "parse adapter json: %v", err)
		}
		list = append(list, single)
	} else {
		if err := dec.Decode(&list); err != nil {
			return nil, newError(CodeCommandFailed, "parse adapter json: %v", err)
		}
	}
	result := make([]Interface, 0, len(list))
	for _, ps := range list {
		iface := Interface{
			Name: ps.Name, Label: ps.Label, Type: ps.Type,
			// netsh/PS report MACs with dashes; colon form everywhere.
			HardwareAddr: strings.ReplaceAll(ps.HardwareAddr, "-", ":"),
			MTU:          ps.MTU, Up: ps.Up,
			DHCP: ps.DHCP, Gateway: ps.Gateway, DNS: ps.DNS,
		}
		for _, ip := range ps.IPs {
			info := IPInfo{Address: ip.Address, Family: ip.Family, Dynamic: ip.Dynamic}
			if ip.Family == "ipv4" {
				if mask, ok := prefixLenToMask(ip.Prefix); ok {
					info.Netmask = mask
				}
			}
			iface.IPs = append(iface.IPs, info)
		}
		result = append(result, iface)
	}
	return result, nil
}

var (
	wlanSSIDRe    = regexp.MustCompile(`SSID\s+\d+\s*:\s*(.*)`)
	wlanSignalRe  = regexp.MustCompile(`(\d+)\s*%`)
	wlanAuthRe    = regexp.MustCompile(`(?i)\b(WPA3?[^:\r\n]*|WPA2[^:\r\n]*|open)\b`)
	wlanBSSIDLine = regexp.MustCompile(`(?i)bssid\s+\d+\s*:\s*((?:[0-9a-fA-F]{2}:){5}[0-9a-fA-F]{2})`)
)

// parseWlanNetworks parses `netsh wlan show networks mode=bssid`. Key labels
// are localized, so SSID lines (which keep the SSID token) and percent
// signals are matched structurally instead of by label name.
func parseWlanNetworks(out string, current string) []WifiNetwork {
	var nets []WifiNetwork
	var block strings.Builder
	flush := func() {
		text := block.String()
		block.Reset()
		m := wlanSSIDRe.FindStringSubmatch(strings.SplitN(text, "\n", 2)[0])
		if m == nil {
			return
		}
		ssid := strings.TrimSpace(m[1])
		if ssid == "" {
			return
		}
		net := WifiNetwork{SSID: ssid, Current: ssid == current}
		if sig := wlanSignalRe.FindStringSubmatch(text); sig != nil {
			net.Signal, _ = strconv.Atoi(sig[1])
		}
		if b := wlanBSSIDLine.FindStringSubmatch(text); b != nil {
			net.BSSID = b[1]
		}
		if auth := wlanAuthRe.FindString(text); auth != "" {
			net.Security = strings.TrimSpace(auth)
		}
		nets = append(nets, net)
	}
	for _, raw := range strings.Split(out, "\n") {
		if wlanSSIDRe.MatchString(raw) && !strings.Contains(raw, "BSSID") {
			flush()
			block.WriteString(raw)
			block.WriteString("\n")
			continue
		}
		if block.Len() > 0 {
			block.WriteString(raw)
			block.WriteString("\n")
		}
	}
	flush()
	return nets
}

// parseWlanCurrentSSID extracts the associated SSID from
// `netsh wlan show interfaces`; returns "" when not connected. The "SSID"
// token stays English on localized Windows; "BSSID"-only lines are skipped.
func parseWlanCurrentSSID(out string) string {
	for _, raw := range strings.Split(out, "\n") {
		trimmed := strings.TrimSpace(raw)
		if strings.Contains(trimmed, "BSSID") {
			continue
		}
		key, val, ok := strings.Cut(trimmed, ":")
		if !ok {
			continue
		}
		key = strings.TrimSpace(key)
		if key == "SSID" && strings.TrimSpace(val) != "" {
			return strings.TrimSpace(val)
		}
	}
	return ""
}

// psProxy mirrors the registry proxy blob read via Get-ItemProperty.
type psProxy struct {
	ProxyEnable   int    `json:"ProxyEnable"`
	ProxyServer   string `json:"ProxyServer"`
	ProxyOverride string `json:"ProxyOverride"`
	AutoConfigURL string `json:"AutoConfigURL"`
}

// parsePSProxy converts the registry blob to ProxyConfig. ProxyServer is
// either "host:port" for all protocols or "http=h:p;https=h:p".
func parsePSProxy(blob string) (ProxyConfig, error) {
	blob = strings.TrimSpace(blob)
	if blob == "" {
		return ProxyConfig{Mode: "none"}, nil
	}
	var ps psProxy
	if err := json.Unmarshal([]byte(blob), &ps); err != nil {
		return ProxyConfig{}, newError(CodeCommandFailed, "parse proxy registry: %v", err)
	}
	cfg := ProxyConfig{Mode: "none"}
	if ps.ProxyEnable != 1 {
		return cfg, nil
	}
	cfg.Mode = "manual"
	httpHost, httpPort, httpsHost, httpsPort := splitProxyServer(ps.ProxyServer)
	cfg.HTTPHost, cfg.HTTPPort, cfg.HTTPSHost, cfg.HTTPSPort = httpHost, httpPort, httpsHost, httpsPort
	for _, part := range strings.FieldsFunc(ps.ProxyOverride, func(r rune) bool { return r == ';' }) {
		if strings.TrimSpace(part) != "" {
			cfg.Bypass = append(cfg.Bypass, strings.TrimSpace(part))
		}
	}
	return cfg, nil
}

// splitProxyServer handles both ProxyServer spellings.
func splitProxyServer(server string) (httpHost string, httpPort int, httpsHost string, httpsPort int) {
	parts := strings.Split(server, ";")
	plain := func(s string) (string, int) {
		host, portStr, _ := strings.Cut(s, ":")
		port, _ := strconv.Atoi(portStr)
		return strings.TrimSpace(host), port
	}
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		if strings.Contains(part, "=") {
			proto, addr, _ := strings.Cut(part, "=")
			host, port := plain(addr)
			switch strings.ToLower(proto) {
			case "http":
				httpHost, httpPort = host, port
			case "https":
				httpsHost, httpsPort = host, port
			}
			continue
		}
		httpHost, httpPort = plain(part)
		httpsHost, httpsPort = httpHost, httpPort
	}
	return
}

// buildProxyServer is the inverse of splitProxyServer for the registry value.
func buildProxyServer(cfg ProxyConfig) string {
	if cfg.HTTPSHost != "" && (cfg.HTTPSHost != cfg.HTTPHost || cfg.HTTPSPort != cfg.HTTPPort) {
		httpPart := cfg.HTTPHost + ":" + strconv.Itoa(cfg.HTTPPort)
		if cfg.HTTPHost == "" {
			httpPart = ""
		}
		httpsPart := cfg.HTTPSHost + ":" + strconv.Itoa(cfg.HTTPSPort)
		if httpPart == "" {
			return httpsPart
		}
		return "http=" + httpPart + ";https=" + httpsPart
	}
	return cfg.HTTPHost + ":" + strconv.Itoa(cfg.HTTPPort)
}

var xmlEscaper = strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;", `"`, "&quot;")

// wlanProfileXML builds a WLAN profile for WPA2-Personal (AES) or open
// networks; SSID and password are XML-escaped.
func wlanProfileXML(ssid, password string) (string, error) {
	if ssid == "" {
		return "", newError(CodeInvalidArgument, "ssid is empty")
	}
	if len(ssid) > 32 {
		return "", newError(CodeInvalidArgument, "ssid longer than 32 bytes")
	}
	auth, enc, shared := "open", "none", ""
	if password != "" {
		auth, enc = "WPA2PSK", "AES"
		shared = `<sharedKey><keyType>passPhrase</keyType><protected>false</protected><keyMaterial>` +
			xmlEscaper.Replace(password) + `</keyMaterial></sharedKey>`
	}
	return fmt.Sprintf(`<?xml version="1.0"?>
<WLANProfile xmlns="http://www.microsoft.com/networking/WLAN/profile/v1">
	<name>%s</name>
	<SSIDConfig><SSID><name>%s</name></SSID></SSIDConfig>
	<connectionType>ESS</connectionType>
	<connectionMode>auto</connectionMode>
	<MSM><security>
		<authEncryption><authentication>%s</authentication><encryption>%s</encryption><useOneX>false</useOneX></authEncryption>
		%s
	</security></MSM>
</WLANProfile>
`, xmlEscaper.Replace(ssid), xmlEscaper.Replace(ssid), auth, enc, shared), nil
}
