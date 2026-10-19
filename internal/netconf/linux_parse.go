package netconf

import (
	"encoding/json"
	"fmt"
	"net"
	"strconv"
	"strings"
)

// Parsers for Linux NetworkManager (nmcli -t), `ip -j` JSON, resolv.conf
// and gsettings GVariant values. Build-tag free so tests run everywhere.

// splitTerse splits an `nmcli -t` line on unescaped colons (nmcli escapes
// colons inside values as "\:").
func splitTerse(line string) []string {
	var fields []string
	var current strings.Builder
	for i := 0; i < len(line); i++ {
		switch line[i] {
		case '\\':
			if i+1 < len(line) {
				current.WriteByte(line[i+1])
				i++
			}
		case ':':
			fields = append(fields, current.String())
			current.Reset()
		default:
			current.WriteByte(line[i])
		}
	}
	fields = append(fields, current.String())
	return fields
}

// unescapeTerse turns nmcli value escaping into the plain value.
func unescapeTerse(v string) string { return strings.ReplaceAll(v, "\\:", ":") }

// nmDevice accumulates one `nmcli -t -f ... device show` block.
type nmDevice struct {
	Device     string
	Type       string
	HWAddr     string
	MTU        int
	State      string
	Connection string
	IP4s       []string // "ip/prefix"
	Gateway    string
	DNS        []string
}

// parseNMDeviceShow parses `nmcli -t -f <fields> device show` output into
// per-device blocks.
func parseNMDeviceShow(out string) []nmDevice {
	var devices []nmDevice
	var current *nmDevice
	flush := func() {
		if current != nil && current.Device != "" {
			devices = append(devices, *current)
		}
		current = nil
	}
	for _, raw := range strings.Split(out, "\n") {
		line := strings.TrimRight(raw, "\r")
		if strings.TrimSpace(line) == "" {
			flush()
			continue
		}
		parts := splitTerse(line)
		if len(parts) < 2 {
			continue
		}
		key, value := parts[0], strings.Join(parts[1:], ":")
		if key == "GENERAL.DEVICE" {
			flush()
			current = &nmDevice{Device: value}
			continue
		}
		if current == nil {
			continue
		}
		switch key {
		case "GENERAL.TYPE":
			current.Type = value
		case "GENERAL.HWADDR":
			current.HWAddr = value
		case "GENERAL.MTU":
			current.MTU, _ = strconv.Atoi(value)
		case "GENERAL.STATE":
			current.State = value
		case "GENERAL.CONNECTION":
			if value != "--" && value != "" {
				current.Connection = value
			}
		case "IP4.GATEWAY":
			if value != "--" && value != "" {
				current.Gateway = value
			}
		case "IP4.DNS", "IP4.DNS[1]", "IP4.DNS[2]", "IP4.DNS[3]", "IP4.DNS[4]", "IP4.DNS[5]":
			if value != "" {
				current.DNS = append(current.DNS, value)
			}
		}
		if strings.HasPrefix(key, "IP4.ADDRESS") && value != "" && value != "--" {
			current.IP4s = append(current.IP4s, value)
		}
	}
	flush()
	return devices
}

// nmDHCP maps device → connection method from
// `nmcli -t -f NAME,DEVICE,ipv4.method connection show`.
func parseNMConnectionMethods(out string) map[string]bool {
	dhcp := map[string]bool{}
	for _, raw := range strings.Split(out, "\n") {
		line := strings.TrimSpace(raw)
		if line == "" {
			continue
		}
		fields := splitTerse(line)
		if len(fields) < 3 {
			continue
		}
		device, method := fields[1], fields[len(fields)-1]
		dhcp[device] = method == "auto"
	}
	return dhcp
}

// nmWifi is one row of `nmcli -t -f ... device wifi list`.
type nmWifi struct {
	SSID     string
	BSSID    string
	Channel  string
	Signal   int
	Security string
	Active   bool
}

// parseNMWifiList parses wifi rows; fields are SSID,BSSID,CHAN,SIGNAL,SECURITY
// and a trailing active/in-use flag.
func parseNMWifiList(out string, hasActiveColumn bool) []WifiNetwork {
	var nets []WifiNetwork
	for _, raw := range strings.Split(out, "\n") {
		line := strings.TrimSpace(raw)
		if line == "" {
			continue
		}
		fields := splitTerse(line)
		if len(fields) < 5 {
			continue
		}
		var w nmWifi
		w.SSID = fields[0]
		w.BSSID = unescapeTerse(fields[1])
		w.Channel = fields[2]
		w.Signal, _ = strconv.Atoi(fields[3])
		w.Security = fields[4]
		if hasActiveColumn && len(fields) >= 6 {
			w.Active = fields[5] == "yes" || fields[5] == "true"
		}
		if w.SSID == "" || w.SSID == "--" {
			continue // hidden networks carry no SSID
		}
		nets = append(nets, WifiNetwork{
			SSID: w.SSID, BSSID: w.BSSID, Signal: w.Signal,
			Security: w.Security, Channel: w.Channel, Current: w.Active,
		})
	}
	return nets
}

// linuxClassify maps an nmcli GENERAL.TYPE to an Interface type.
func linuxClassify(t string) string {
	switch t {
	case "wifi":
		return "wifi"
	case "loopback":
		return "loopback"
	case "ethernet", "bridge", "tun", "tap", "veth", "bond":
		return "ethernet"
	default:
		return "other"
	}
}

// gvariantUnquote strips GVariant string quoting: 'none' → none.
func gvariantUnquote(v string) string {
	v = strings.TrimSpace(v)
	v = strings.TrimPrefix(v, "'")
	v = strings.TrimSuffix(v, "'")
	v = strings.TrimPrefix(v, `"`)
	v = strings.TrimSuffix(v, `"`)
	return v
}

// gvariantArray parses ['a', 'b'] into ["a", "b"].
func gvariantArray(v string) []string {
	v = strings.TrimSpace(v)
	v = strings.TrimPrefix(v, "[")
	v = strings.TrimSuffix(v, "]")
	var items []string
	for _, part := range strings.Split(v, ",") {
		if s := gvariantUnquote(part); s != "" {
			items = append(items, s)
		}
	}
	return items
}

// gvariantArrayString formats strings back into GVariant array syntax.
func gvariantArrayString(items []string) string {
	if len(items) == 0 {
		return "@as []"
	}
	quoted := make([]string, len(items))
	for i, item := range items {
		quoted[i] = "'" + strings.ReplaceAll(item, "'", `\'`) + "'"
	}
	return "[" + strings.Join(quoted, ", ") + "]"
}

// ipAddrEntry mirrors one `ip -j addr` object.
type ipAddrEntry struct {
	Ifname    string   `json:"ifname"`
	Flags     []string `json:"flags"`
	MTU       int      `json:"mtu"`
	Operstate string   `json:"operstate"`
	Address   string   `json:"address"` // hardware address
	AddrInfo  []struct {
		Family    string `json:"family"`
		Local     string `json:"local"`
		Prefixlen int    `json:"prefixlen"`
	} `json:"addr_info"`
}

// ipRouteEntry mirrors one `ip -j route` object.
type ipRouteEntry struct {
	Dst     string `json:"dst"`
	Dev     string `json:"dev"`
	Gateway string `json:"gateway"`
}

// parseIPAddr converts `ip -j addr` output into Interface values.
func parseIPAddr(out string) ([]Interface, error) {
	var entries []ipAddrEntry
	if err := json.Unmarshal([]byte(out), &entries); err != nil {
		return nil, newError(CodeCommandFailed, "parse ip addr json: %v", err)
	}
	result := make([]Interface, 0, len(entries))
	for _, e := range entries {
		iface := Interface{
			Name: e.Ifname, Label: e.Ifname, MTU: e.MTU, HardwareAddr: e.Address,
		}
		iface.Type = "ethernet"
		if e.Ifname == "lo" {
			iface.Type = "loopback"
		}
		if strings.HasPrefix(e.Ifname, "wl") || strings.HasPrefix(e.Ifname, "wlan") {
			iface.Type = "wifi"
		}
		for _, f := range e.Flags {
			if f == "UP" {
				iface.Up = true
			}
		}
		if e.Operstate == "up" || e.Operstate == "unknown" {
			iface.Up = true
		}
		for _, a := range e.AddrInfo {
			info := IPInfo{Address: a.Local}
			if a.Family == "inet" {
				info.Family = "ipv4"
				if mask, ok := prefixLenToMask(a.Prefixlen); ok {
					info.Netmask = mask
				}
			} else {
				info.Family = "ipv6"
			}
			if a.Local == "127.0.0.1" || strings.HasPrefix(a.Local, "fe80:") {
				continue
			}
			iface.IPs = append(iface.IPs, info)
		}
		result = append(result, iface)
	}
	return result, nil
}

// parseIPRoute returns device → default gateway from `ip -j route show default`.
func parseIPRoute(out string) map[string]string {
	gateways := map[string]string{}
	var entries []ipRouteEntry
	if err := json.Unmarshal([]byte(out), &entries); err != nil {
		return gateways
	}
	for _, e := range entries {
		if e.Dev != "" && e.Gateway != "" {
			if _, ok := gateways[e.Dev]; !ok {
				gateways[e.Dev] = e.Gateway
			}
		}
	}
	return gateways
}

// parseResolvConf pulls nameserver lines out of resolv.conf content.
func parseResolvConf(content string) []string {
	var servers []string
	for _, raw := range strings.Split(content, "\n") {
		line := strings.TrimSpace(raw)
		rest, ok := strings.CutPrefix(line, "nameserver")
		if !ok {
			continue
		}
		for _, field := range strings.Fields(rest) {
			if net.ParseIP(field) != nil {
				servers = append(servers, field)
			}
		}
	}
	return servers
}

// ipWithPrefix joins "192.168.1.5" and 24 into "192.168.1.5/24".
func ipWithPrefix(ip string, prefix int) string {
	return fmt.Sprintf("%s/%d", ip, prefix)
}

// splitCIDR splits "192.168.1.5/24" into its parts.
func splitCIDR(s string) (string, int, bool) {
	ip, prefixStr, ok := strings.Cut(s, "/")
	if !ok || net.ParseIP(ip) == nil {
		return "", 0, false
	}
	prefix, err := strconv.Atoi(prefixStr)
	if err != nil {
		return "", 0, false
	}
	return ip, prefix, true
}
