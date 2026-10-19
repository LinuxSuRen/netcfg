package netconf

import (
	"net"
	"regexp"
	"strconv"
	"strings"
)

// Parsers for macOS `networksetup` / `ifconfig` / `airport` output. They are
// deliberately build-tag free so table-driven tests run on every platform.

// parseDarwinServices parses `networksetup -listallnetworkservices`; a
// leading asterisk marks a disabled service and is stripped.
func parseDarwinServices(out string) []string {
	var services []string
	for _, raw := range strings.Split(out, "\n") {
		line := strings.TrimSpace(raw)
		if line == "" || strings.HasPrefix(line, "An asterisk") {
			continue
		}
		services = append(services, strings.TrimPrefix(line, "*"))
	}
	return services
}

// darwinServiceInfo is the per-service device mapping from
// `networksetup -listnetworkserviceorder`.
type darwinServiceInfo struct {
	Service string
	Device  string
	Port    string
}

// parseDarwinServiceOrder pairs service names with their hardware port and
// device, e.g. "(1) Wi-Fi" followed by "(Hardware Port: Wi-Fi, Device: en0, ...)".
func parseDarwinServiceOrder(out string) []darwinServiceInfo {
	var result []darwinServiceInfo
	var current string
	for _, raw := range strings.Split(out, "\n") {
		line := strings.TrimSpace(raw)
		if line == "" {
			continue
		}
		if strings.HasPrefix(line, "(") && strings.Contains(line, "Hardware Port:") {
			var info darwinServiceInfo
			info.Service = current
			for _, field := range strings.Split(strings.Trim(line, "()"), ",") {
				key, val, ok := strings.Cut(strings.TrimSpace(field), ":")
				if !ok {
					continue
				}
				switch strings.TrimSpace(key) {
				case "Hardware Port":
					info.Port = strings.TrimSpace(val)
				case "Device":
					info.Device = strings.TrimSpace(val)
				}
			}
			if info.Service != "" && info.Device != "" {
				result = append(result, info)
			}
			current = ""
			continue
		}
		// "(1) Wi-Fi" — service entry line.
		inner := strings.TrimPrefix(line, "(")
		if idx := strings.Index(inner, ")"); idx >= 0 {
			current = strings.TrimSpace(inner[idx+1:])
		}
	}
	return result
}

// parseDarwinGetInfo parses `networksetup -getinfo <service>` into address
// facts. IPv4 netmasks are matched to addresses by order, matching how the
// tool prints them.
func parseDarwinGetInfo(out string) (dhcp bool, ipv4s, netmasks []string, gateway string, ipv6s []string) {
	for _, raw := range strings.Split(out, "\n") {
		line := strings.TrimSpace(raw)
		switch {
		case line == "DHCP Configuration", strings.HasPrefix(line, "Using DHCP"):
			dhcp = true
		case line == "Manual Configuration":
			dhcp = false
		}
		key, val, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		key, val = strings.TrimSpace(key), strings.TrimSpace(val)
		if val == "" {
			continue
		}
		switch key {
		case "IP address", "IP Addresses (IPv4)":
			for _, field := range strings.Fields(val) {
				if ip := net.ParseIP(field); ip != nil && ip.To4() != nil {
					ipv4s = append(ipv4s, field)
				}
			}
		case "Subnet mask", "Subnet Masks (IPv4)":
			for _, field := range strings.Fields(val) {
				if ip := net.ParseIP(field); ip != nil && ip.To4() != nil {
					netmasks = append(netmasks, field)
				}
			}
		case "Router", "Router (IPv4)":
			if ip := net.ParseIP(val); ip != nil && ip.To4() != nil {
				gateway = val
			}
		case "IPv6 IP address", "IP Addresses (IPv6)":
			for _, field := range strings.Fields(val) {
				if ip := net.ParseIP(field); ip != nil && ip.To4() == nil {
					ipv6s = append(ipv6s, field)
				}
			}
		}
	}
	return dhcp, ipv4s, netmasks, gateway, ipv6s
}

// parseDarwinDNS parses `networksetup -getdnsservers <service>`.
func parseDarwinDNS(out string) []string {
	var servers []string
	for _, raw := range strings.Split(out, "\n") {
		line := strings.TrimSpace(raw)
		if line == "" || strings.Contains(line, "There aren't any DNS") {
			continue
		}
		for _, field := range strings.Fields(line) {
			if net.ParseIP(field) != nil {
				servers = append(servers, field)
			}
		}
	}
	return servers
}

// darwinLink is the per-device link state parsed from `ifconfig`.
type darwinLink struct {
	Up    bool
	MTU   int
	Ether string
}

var darwinIfconfigRe = regexp.MustCompile(`^(\S+?): flags=\d+(?:<([^>]*)>)? mtu (\d+)`)

// parseDarwinIfconfig maps device name to link state from `ifconfig` output,
// including the hardware address from the block's `ether` line.
func parseDarwinIfconfig(out string) map[string]darwinLink {
	links := map[string]darwinLink{}
	current := ""
	for _, raw := range strings.Split(out, "\n") {
		if m := darwinIfconfigRe.FindStringSubmatch(raw); m != nil {
			current = m[1]
			mtu, _ := strconv.Atoi(m[3])
			up := false
			for _, f := range strings.Split(m[2], ",") {
				if f == "UP" {
					up = true
				}
			}
			links[current] = darwinLink{Up: up, MTU: mtu}
			continue
		}
		if current == "" {
			continue
		}
		line := strings.TrimSpace(raw)
		if rest, ok := strings.CutPrefix(line, "ether "); ok {
			link := links[current]
			link.Ether = strings.TrimSpace(rest)
			links[current] = link
		}
	}
	return links
}

// parseDarwinAirportNetwork parses `networksetup -getairportnetwork <svc>`;
// returns "" when not associated.
func parseDarwinAirportNetwork(out string) string {
	const prefix = "Current Wi-Fi Network:"
	line := strings.TrimSpace(out)
	if rest, ok := strings.CutPrefix(line, prefix); ok {
		return strings.TrimSpace(rest)
	}
	return ""
}

var bssidRe = regexp.MustCompile(`(?:[0-9a-fA-F]{2}:){5}[0-9a-fA-F]{2}`)

// parseAirportScan parses `airport -s` output. SSIDs and security strings may
// contain spaces, so lines are split around the BSSID column.
func parseAirportScan(out string) []WifiNetwork {
	var nets []WifiNetwork
	for _, raw := range strings.Split(out, "\n") {
		line := raw
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "SSID") {
			continue // header or blank
		}
		loc := bssidRe.FindStringIndex(line)
		if loc == nil {
			continue
		}
		ssid := strings.TrimSpace(line[:loc[0]])
		fields := strings.Fields(line[loc[1]:])
		if len(fields) < 4 || ssid == "" {
			continue // RSSI CHANNEL HT CC SECURITY
		}
		rssi, _ := strconv.Atoi(fields[0])
		security := ""
		if len(fields) > 4 {
			security = strings.Join(fields[4:], " ")
			if security == "--" {
				security = ""
			}
		}
		nets = append(nets, WifiNetwork{
			SSID:     ssid,
			BSSID:    line[loc[0]:loc[1]],
			Signal:   rssiToPercent(rssi),
			Channel:  fields[1],
			Security: security,
		})
	}
	return nets
}

// rssiToPercent maps dBm (-100..0) onto a 0-100 quality scale.
func rssiToPercent(rssi int) int {
	switch {
	case rssi <= -100:
		return 0
	case rssi >= -50:
		return 100
	default:
		pct := 2 * (rssi + 100)
		if pct > 100 {
			return 100
		}
		return pct
	}
}

// parseDarwinProxy parses `networksetup -getwebproxy`/`-getsecurewebproxy`.
func parseDarwinProxy(out string) (enabled bool, host string, port int) {
	for _, raw := range strings.Split(out, "\n") {
		key, val, ok := strings.Cut(strings.TrimSpace(raw), ":")
		if !ok {
			continue
		}
		switch strings.TrimSpace(key) {
		case "Enabled":
			enabled = strings.EqualFold(strings.TrimSpace(val), "yes")
		case "Server":
			host = strings.TrimSpace(val)
		case "Port":
			port, _ = strconv.Atoi(strings.TrimSpace(val))
		}
	}
	return enabled, host, port
}

// parseDarwinBypass parses `networksetup -getproxybypassdomains`.
func parseDarwinBypass(out string) []string {
	var domains []string
	for _, raw := range strings.Split(out, "\n") {
		line := strings.TrimSpace(raw)
		if line == "" || strings.Contains(line, "There aren't any") {
			continue
		}
		domains = append(domains, line)
	}
	return domains
}

// darwinClassify maps a hardware port / device name to an Interface type.
func darwinClassify(port, device string) string {
	name := port
	if name == "" {
		name = device
	}
	switch {
	case strings.Contains(name, "Wi-Fi"), strings.Contains(name, "AirPort"):
		return "wifi"
	case device == "lo":
		return "loopback"
	case strings.Contains(name, "Ethernet"), strings.Contains(name, "Thunderbolt"),
		strings.Contains(name, "USB"), strings.Contains(name, "LAN"),
		strings.Contains(name, "Bridge"), strings.HasPrefix(device, "en"),
		strings.HasPrefix(device, "bridge"), strings.HasPrefix(device, "utun"):
		return "ethernet"
	default:
		return "other"
	}
}

// maskToPrefixLen converts a dotted netmask to a prefix length; ok is false
// for malformed input.
func maskToPrefixLen(mask string) (int, bool) {
	ip := net.ParseIP(mask)
	if ip == nil || ip.To4() == nil {
		return 0, false
	}
	ones, bits := net.IPMask(ip.To4()).Size()
	if bits != 32 {
		return 0, false
	}
	return ones, true
}

// prefixLenToMask is the inverse of maskToPrefixLen.
func prefixLenToMask(ones int) (string, bool) {
	if ones < 0 || ones > 32 {
		return "", false
	}
	m := net.CIDRMask(ones, 32)
	return net.IP(m).String(), true
}

// spNetworkNameRe matches a bare "Name:" line at network-entry depth
// (12 spaces in system_profiler output; property lines carry values).
var spNetworkNameRe = regexp.MustCompile(`^\s{11,}(\S[^:]*):\s*$`)

// parseSystemProfilerAirport parses `system_profiler SPAirPortDataType`,
// the Wi-Fi listing that works on modern macOS where the deprecated
// airport tool does not. The listing is nested inside per-interface blocks
// (e.g. en0), so parsing follows the indentation: network entries sit at
// depth 12+ under their section markers; anything back at depth <= 10
// leaves the section. Redacted SSIDs are counted, not returned.
func parseSystemProfilerAirport(out string) (current string, nets []WifiNetwork, redacted int) {
	section := "" // "" | current | other
	var name string
	var channel, security string
	var rssi int
	have := false
	flush := func() {
		if !have {
			return
		}
		have = false
		if strings.Contains(name, "redacted") {
			redacted++
			return
		}
		sec := security
		if strings.EqualFold(sec, "None") {
			sec = ""
		}
		nets = append(nets, WifiNetwork{
			SSID: name, Channel: channel, Security: sec,
			Signal: rssiToPercent(rssi), Current: section == "current",
		})
	}
	for _, raw := range strings.Split(out, "\n") {
		line := strings.TrimRight(raw, " \r")
		if strings.TrimSpace(line) == "" {
			continue
		}
		if strings.Contains(line, "Current Network Information") {
			flush()
			section = "current"
			continue
		}
		if strings.Contains(line, "Wi-Fi Networks:") {
			flush()
			section = "other"
			continue
		}
		if section == "" {
			continue
		}
		depth := len(line) - len(strings.TrimLeft(line, " "))
		if depth <= 10 {
			// Left the network listing (next interface block or sibling
			// section such as "Interfaces:" / "awdl0:").
			flush()
			section = ""
			continue
		}
		if m := spNetworkNameRe.FindStringSubmatch(line); m != nil {
			flush()
			name, channel, security, rssi = strings.TrimSpace(m[1]), "", "", -100
			have = true
			continue
		}
		key, val, ok := strings.Cut(strings.TrimSpace(line), ":")
		if !ok || !have {
			continue
		}
		key, val = strings.TrimSpace(key), strings.TrimSpace(val)
		switch key {
		case "Channel":
			if fields := strings.Fields(val); len(fields) > 0 {
				channel = fields[0]
			}
		case "Security":
			security = val
		case "Signal / Noise":
			if fields := strings.Fields(val); len(fields) > 0 {
				rssi, _ = strconv.Atoi(fields[0])
			}
		}
	}
	flush()
	for i := range nets {
		if nets[i].Current {
			current = nets[i].SSID
		}
	}
	return current, nets, redacted
}

// parseWdutilInfo parses `wdutil info`, Apple's Wi-Fi diagnostics dump that
// still reports the current SSID under sudo on macOS 15 where other tools
// are redacted. Returns nil when the SSID is hidden or absent.
func parseWdutilInfo(out string) *WifiNetwork {
	var net WifiNetwork
	haveSSID := false
	for _, raw := range strings.Split(out, "\n") {
		key, val, ok := strings.Cut(strings.TrimSpace(raw), ":")
		if !ok {
			continue
		}
		key, val = strings.TrimSpace(key), strings.TrimSpace(val)
		switch key {
		case "SSID":
			if val != "" && !strings.Contains(val, "redacted") {
				net.SSID, haveSSID = val, true
			}
		case "BSSID":
			net.BSSID = val
		case "CHANNEL":
			if fields := strings.Fields(val); len(fields) > 0 {
				net.Channel = fields[0]
			}
		case "RSSI":
			if fields := strings.Fields(val); len(fields) > 0 {
				rssi, _ := strconv.Atoi(fields[0])
				net.Signal = rssiToPercent(rssi)
			}
		}
	}
	if !haveSSID {
		return nil
	}
	net.Current = true
	return &net
}
