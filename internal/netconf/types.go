// Package netconf provides a platform-agnostic view of – and control over –
// the local machine's network configuration (interfaces, IP/DHCP, DNS,
// Wi-Fi, system proxy, hosts file) on macOS, Windows and Linux.
package netconf

// Interface is one configurable network interface as exposed by the CLI and
// the Web API. Name is the stable identifier used by every mutating command
// and equals the platform's own notion of the unit: the network service name
// on macOS (e.g. "Wi-Fi"), the interface alias on Windows (e.g. "Ethernet")
// and the device name on Linux (e.g. "wlan0").
type Interface struct {
	Name         string   `json:"name"`
	Label        string   `json:"label,omitempty"`
	Type         string   `json:"type"` // ethernet | wifi | loopback | other
	HardwareAddr string   `json:"hardware_addr,omitempty"`
	MTU          int      `json:"mtu,omitempty"`
	Up           bool     `json:"up"`
	IPs          []IPInfo `json:"ips"`
	DHCP         bool     `json:"dhcp"`
	Gateway      string   `json:"gateway,omitempty"`
	DNS          []string `json:"dns,omitempty"`
	SSID         string   `json:"ssid,omitempty"`
}

// IPInfo is a single address assigned to an interface.
type IPInfo struct {
	Address string `json:"address"` // dotted quad, IPv6 as Go formats it
	Netmask string `json:"netmask,omitempty"`
	Family  string `json:"family"` // ipv4 | ipv6
	Dynamic bool   `json:"dynamic"`
}

// IPConfig describes how an interface should get its IPv4 address: either
// DHCP (fields ignored) or a static triple.
type IPConfig struct {
	DHCP    bool   `json:"dhcp"`
	IP      string `json:"ip,omitempty"`
	Netmask string `json:"netmask,omitempty"`
	Gateway string `json:"gateway,omitempty"`
}

// DNSConfig sets resolver servers; an empty/cleared list returns the
// interface to automatic (DHCP-provided) DNS.
type DNSConfig struct {
	Auto    bool     `json:"auto"` // true clears manual servers
	Servers []string `json:"servers,omitempty"`
}

// WifiNetwork is one visible wireless network.
type WifiNetwork struct {
	SSID     string `json:"ssid"`
	BSSID    string `json:"bssid,omitempty"`
	Signal   int    `json:"signal"` // 0-100
	Security string `json:"security,omitempty"`
	Channel  string `json:"channel,omitempty"`
	Current  bool   `json:"current,omitempty"`
}

// WifiAPOptions tunes a hotspot started via StartWifiAP. Zero values keep
// platform defaults.
type WifiAPOptions struct {
	Channel  int    // 0 = auto-select
	Band     string // "bg" (2.4 GHz, default) | "a" (5 GHz)
	Gateway  string // hotspot LAN gateway IPv4; "" = platform default
	ConnName string // connection/profile name; "" = "netcfg-ap"
}

// ProxyConfig is the effective system (desktop) proxy: either none or an
// explicit HTTP/HTTPS proxy with a bypass list.
type ProxyConfig struct {
	Mode      string   `json:"mode"` // none | manual
	HTTPHost  string   `json:"http_host,omitempty"`
	HTTPPort  int      `json:"http_port,omitempty"`
	HTTPSHost string   `json:"https_host,omitempty"`
	HTTPSPort int      `json:"https_port,omitempty"`
	Bypass    []string `json:"bypass,omitempty"`
}

// Capabilities tells the UI which operations the current platform backend
// actually supports, so it can hide or disable the rest instead of failing.
type Capabilities struct {
	SetIP    bool `json:"set_ip"`
	SetDNS   bool `json:"set_dns"`
	WifiScan bool `json:"wifi_scan"`
	WifiJoin bool `json:"wifi_join"`
	WifiAP   bool `json:"wifi_ap"`
	Proxy    bool `json:"proxy"`
	Hosts    bool `json:"hosts"`
}

// HostEntry is one line of the hosts file. A line may map several hostnames
// to one address; managed entries are the ones netcfg owns (see hosts.go).
type HostEntry struct {
	IP        string   `json:"ip"`
	Hostnames []string `json:"hostnames"`
	Managed   bool     `json:"managed"`
}
