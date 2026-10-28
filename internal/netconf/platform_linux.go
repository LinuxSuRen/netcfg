//go:build linux

package netconf

import (
	"net"
	"os/exec"
	"strings"
)

// linuxManager implements Manager through NetworkManager's nmcli when
// present; without NetworkManager it degrades to a read-only view built
// from iproute2 and resolv.conf, and mutating operations report
// unsupported. The desktop proxy goes through gsettings (GNOME).
type linuxManager struct {
	query  Runner
	mutate Runner
	hasNM  bool
}

// NewManager returns the Linux backend, detecting NetworkManager.
func NewManager() Manager {
	_, err := exec.LookPath("nmcli")
	return &linuxManager{query: ExecRunner{}, mutate: ExecRunner{}, hasNM: err == nil}
}

// NewDryManager returns the Linux backend recording mutations instead of
// running them.
func NewDryManager() Manager {
	_, err := exec.LookPath("nmcli")
	return &linuxManager{query: ExecRunner{}, mutate: &DryRunner{}, hasNM: err == nil}
}

// DryCommands returns the recorded mutations (used by the CLI dry run).
func (m *linuxManager) DryCommands() []string {
	if d, ok := m.mutate.(*DryRunner); ok {
		return d.Commands
	}
	return nil
}

func (m *linuxManager) Capabilities() Capabilities {
	return Capabilities{
		SetIP:    m.hasNM,
		SetDNS:   m.hasNM,
		WifiScan: m.hasNM,
		WifiJoin: m.hasNM,
		WifiAP:   m.hasNM,
		Proxy:    hasGsettings(),
		Hosts:    true,
	}
}

func hasGsettings() bool {
	_, err := exec.LookPath("gsettings")
	return err == nil
}

func (m *linuxManager) Interfaces() ([]Interface, error) {
	if m.hasNM {
		return m.interfacesNM()
	}
	return m.interfacesFallback()
}

func (m *linuxManager) interfacesNM() ([]Interface, error) {
	out, err := m.query.Run("nmcli", "-t", "-e", "no",
		"-f", "GENERAL.DEVICE,GENERAL.TYPE,GENERAL.HWADDR,GENERAL.MTU,GENERAL.STATE,GENERAL.CONNECTION,IP4.ADDRESS,IP4.GATEWAY,IP4.DNS",
		"device", "show")
	if err != nil {
		return nil, err
	}
	devices := parseNMDeviceShow(out)

	dhcpByDevice := map[string]bool{}
	if out, err := m.query.Run("nmcli", "-t", "-f", "NAME,DEVICE,ipv4.method", "connection", "show"); err == nil {
		dhcpByDevice = parseNMConnectionMethods(out)
	}

	activeSSID := m.activeWifiSSID()

	result := make([]Interface, 0, len(devices))
	for _, dev := range devices {
		iface := Interface{
			Name: dev.Device, Label: dev.Device,
			Type:         linuxClassify(dev.Type),
			HardwareAddr: strings.ToLower(dev.HWAddr),
			MTU:          dev.MTU,
			DHCP:         dhcpByDevice[dev.Device],
			Gateway:      dev.Gateway,
			DNS:          dev.DNS,
		}
		iface.Up = strings.Contains(dev.State, "connected") || strings.Contains(dev.State, "connecting")
		for _, addr := range dev.IP4s {
			ip, prefix, ok := splitCIDR(addr)
			if !ok {
				continue
			}
			info := IPInfo{Address: ip, Family: "ipv4", Dynamic: iface.DHCP}
			if mask, ok := prefixLenToMask(prefix); ok {
				info.Netmask = mask
			}
			iface.IPs = append(iface.IPs, info)
		}
		if iface.Type == "wifi" && activeSSID != "" {
			iface.SSID = activeSSID
		}
		result = append(result, iface)
	}
	return result, nil
}

// activeWifiSSID returns the SSID of the active wifi connection.
func (m *linuxManager) activeWifiSSID() string {
	out, err := m.query.Run("nmcli", "-t", "-e", "no", "-f", "ACTIVE,SSID", "device", "wifi")
	if err != nil {
		return ""
	}
	for _, raw := range strings.Split(out, "\n") {
		fields := splitTerse(strings.TrimSpace(raw))
		if len(fields) >= 2 && fields[0] == "yes" && fields[1] != "" {
			return fields[1]
		}
	}
	return ""
}

func (m *linuxManager) interfacesFallback() ([]Interface, error) {
	out, err := m.query.Run("ip", "-j", "addr")
	if err != nil {
		return nil, newError(CodeUnsupported, "NetworkManager not found and `ip -j addr` failed: %v", err)
	}
	ifaces, err := parseIPAddr(out)
	if err != nil {
		return nil, err
	}
	gateways := map[string]string{}
	if out, err := m.query.Run("ip", "-j", "route", "show", "default"); err == nil {
		gateways = parseIPRoute(out)
	}
	dns := readResolvConfDNS(m)
	for i := range ifaces {
		ifaces[i].Gateway = gateways[ifaces[i].Name]
		ifaces[i].DNS = dns
	}
	return ifaces, nil
}

func readResolvConfDNS(m *linuxManager) []string {
	out, err := m.query.Run("cat", "/etc/resolv.conf")
	if err != nil {
		return nil
	}
	return parseResolvConf(out)
}

// connectionOf maps a device to its NetworkManager connection name.
func (m *linuxManager) connectionOf(device string) (string, error) {
	out, err := m.query.Run("nmcli", "-t", "-e", "no",
		"-f", "GENERAL.DEVICE,GENERAL.CONNECTION", "device", "show", device)
	if err != nil {
		return "", err
	}
	for _, raw := range strings.Split(out, "\n") {
		fields := splitTerse(strings.TrimSpace(raw))
		if len(fields) >= 2 && fields[0] == "GENERAL.CONNECTION" && fields[1] != "" && fields[1] != "--" {
			return fields[1], nil
		}
	}
	return "", newError(CodeUnsupported, "device %s has no NetworkManager connection", device)
}

func (m *linuxManager) requireDevice(name string) error {
	ifaces, err := m.Interfaces()
	if err != nil {
		return err
	}
	for _, iface := range ifaces {
		if iface.Name == name {
			return nil
		}
	}
	return ifaceNotFound(name)
}

func (m *linuxManager) SetIP(name string, cfg IPConfig) error {
	if !m.hasNM {
		return newError(CodeUnsupported, "NetworkManager (nmcli) not found; cannot change IP on this system")
	}
	if err := m.requireDevice(name); err != nil {
		return err
	}
	connection, err := m.connectionOf(name)
	if err != nil {
		return err
	}
	args := []string{"connection", "modify", connection}
	if cfg.DHCP {
		args = append(args, "ipv4.method", "auto", "ipv4.addresses", "", "ipv4.gateway", "")
		return errNil(m.mutate.Run("nmcli", args...))
	}
	ip := net.ParseIP(cfg.IP)
	if ip == nil || ip.To4() == nil {
		return newError(CodeInvalidArgument, "%q is not an IPv4 address", cfg.IP)
	}
	prefix, ok := maskToPrefixLen(cfg.Netmask)
	if !ok {
		return newError(CodeInvalidArgument, "%q is not a valid netmask", cfg.Netmask)
	}
	args = append(args, "ipv4.method", "manual", "ipv4.addresses", ipWithPrefix(cfg.IP, prefix))
	if cfg.Gateway != "" {
		if gw := net.ParseIP(cfg.Gateway); gw == nil || gw.To4() == nil {
			return newError(CodeInvalidArgument, "%q is not an IPv4 address", cfg.Gateway)
		}
		args = append(args, "ipv4.gateway", cfg.Gateway)
	} else {
		args = append(args, "ipv4.gateway", "")
	}
	if err := errNil(m.mutate.Run("nmcli", args...)); err != nil {
		return err
	}
	return errNil(m.mutate.Run("nmcli", "connection", "up", connection))
}

func (m *linuxManager) SetDNS(name string, cfg DNSConfig) error {
	if !m.hasNM {
		return newError(CodeUnsupported, "NetworkManager (nmcli) not found; cannot change DNS on this system")
	}
	if err := m.requireDevice(name); err != nil {
		return err
	}
	connection, err := m.connectionOf(name)
	if err != nil {
		return err
	}
	args := []string{"connection", "modify", connection}
	if cfg.Auto {
		args = append(args, "ipv4.ignore-auto-dns", "no", "ipv4.dns", "")
	} else {
		if len(cfg.Servers) == 0 {
			return newError(CodeInvalidArgument, "dns servers list is empty (use auto to clear)")
		}
		for _, s := range cfg.Servers {
			if net.ParseIP(s) == nil {
				return newError(CodeInvalidArgument, "%q is not an IP address", s)
			}
		}
		args = append(args, "ipv4.ignore-auto-dns", "yes", "ipv4.dns", strings.Join(cfg.Servers, ","))
	}
	if err := errNil(m.mutate.Run("nmcli", args...)); err != nil {
		return err
	}
	return errNil(m.mutate.Run("nmcli", "connection", "up", connection))
}

func (m *linuxManager) ScanWifi() ([]WifiNetwork, error) {
	if !m.hasNM {
		return nil, newError(CodeUnsupported, "NetworkManager (nmcli) not found; Wi-Fi scanning is unavailable")
	}
	// ACTIVE replaced INUSE in newer nmcli; try the modern field first.
	out, err := m.query.Run("nmcli", "-t", "-e", "no",
		"-f", "SSID,BSSID,CHAN,SIGNAL,SECURITY,ACTIVE", "device", "wifi", "list")
	if err != nil {
		out, err = m.query.Run("nmcli", "-t", "-e", "no",
			"-f", "SSID,BSSID,CHAN,SIGNAL,SECURITY,INUSE", "device", "wifi", "list")
		if err != nil {
			return nil, err
		}
		return parseNMWifiList(out, true), nil
	}
	return parseNMWifiList(out, true), nil
}

func (m *linuxManager) ConnectWifi(iface, ssid, password string) error {
	if !m.hasNM {
		return newError(CodeUnsupported, "NetworkManager (nmcli) not found; joining Wi-Fi is unavailable")
	}
	if !validSSID(ssid) {
		return newError(CodeInvalidArgument, "invalid SSID %q", ssid)
	}
	args := []string{"device", "wifi", "connect", ssid}
	if password != "" {
		args = append(args, "password", password)
	}
	if iface != "" {
		args = append(args, "ifname", iface)
	}
	_, err := m.mutate.Run("nmcli", args...)
	return err
}

// apConnectionName is the NetworkManager connection netcfg owns for hotspots.
func apConnectionName(opts WifiAPOptions) string {
	if opts.ConnName != "" {
		return opts.ConnName
	}
	return "netcfg-ap"
}

// defaultWifiDevice picks the first wifi interface for hotspot bring-up.
func (m *linuxManager) defaultWifiDevice() (string, error) {
	ifaces, err := m.Interfaces()
	if err != nil {
		return "", err
	}
	for _, iface := range ifaces {
		if iface.Type == "wifi" {
			return iface.Name, nil
		}
	}
	return "", newError(CodeNotFound, "no Wi-Fi interface found; plug in an adapter and check `netcfg list`")
}

func (m *linuxManager) StartWifiAP(iface, ssid, password string, opts WifiAPOptions) error {
	if !m.hasNM {
		return newError(CodeUnsupported, "NetworkManager (nmcli) not found; starting a hotspot is unavailable")
	}
	if err := validateWifiAPArgs(ssid, password, opts); err != nil {
		return err
	}
	if iface == "" {
		var err error
		if iface, err = m.defaultWifiDevice(); err != nil {
			return err
		}
	} else if err := m.requireDevice(iface); err != nil {
		return err
	}
	conn := apConnectionName(opts)
	// Replace a leftover hotspot profile of the same name so consecutive
	// starts converge instead of failing on the existing connection.
	if m.connectionExists(conn) {
		if _, err := m.mutate.Run("nmcli", "connection", "delete", conn); err != nil {
			return err
		}
	}
	args := []string{"connection", "add", "type", "wifi", "ifname", iface,
		"con-name", conn, "ssid", ssid,
		"802-11-wireless.mode", "ap",
		"802-11-wireless.band", "bg"}
	if opts.Band == "a" {
		args[len(args)-1] = "a"
	}
	if opts.Channel != 0 {
		args = append(args, "802-11-wireless.channel", itoa(opts.Channel))
	}
	if password != "" {
		args = append(args,
			"802-11-wireless-security.key-mgmt", "wpa-psk",
			"wifi-sec.psk", password)
	}
	args = append(args, "ipv4.method", "shared")
	if opts.Gateway != "" {
		args = append(args, "ipv4.addresses", opts.Gateway+"/24")
	}
	if _, err := m.mutate.Run("nmcli", args...); err != nil {
		return err
	}
	return errNil(m.mutate.Run("nmcli", "connection", "up", conn))
}

// connectionExists reports whether NetworkManager knows a connection name.
func (m *linuxManager) connectionExists(conn string) bool {
	out, err := m.query.Run("nmcli", "-t", "-f", "NAME", "connection", "show")
	if err != nil {
		return false
	}
	for _, name := range strings.Split(out, "\n") {
		if strings.TrimSpace(name) == conn {
			return true
		}
	}
	return false
}

func (m *linuxManager) StopWifiAP(opts WifiAPOptions) error {
	if !m.hasNM {
		return newError(CodeUnsupported, "NetworkManager (nmcli) not found; stopping a hotspot is unavailable")
	}
	conn := apConnectionName(opts)
	if !m.connectionExists(conn) {
		return newError(CodeNotFound, "no hotspot connection %q (was it started with netcfg?)", conn)
	}
	if _, err := m.mutate.Run("nmcli", "connection", "down", conn); err != nil {
		return err
	}
	return errNil(m.mutate.Run("nmcli", "connection", "delete", conn))
}

const gsettingsProxyBase = "org.gnome.system.proxy"

func (m *linuxManager) Proxy(iface string) (ProxyConfig, error) {
	_ = iface // GNOME proxy is session-global
	if !hasGsettings() {
		return ProxyConfig{}, newError(CodeUnsupported, "gsettings not found; no desktop proxy to read")
	}
	cfg := ProxyConfig{Mode: "none"}
	modeRaw, err := m.query.Run("gsettings", "get", gsettingsProxyBase, "mode")
	if err != nil {
		return ProxyConfig{}, err
	}
	if gvariantUnquote(modeRaw) != "manual" {
		return cfg, nil
	}
	cfg.Mode = "manual"
	if host, err := m.query.Run("gsettings", "get", gsettingsProxyBase+".http", "host"); err == nil {
		cfg.HTTPHost = gvariantUnquote(host)
	}
	if port, err := m.query.Run("gsettings", "get", gsettingsProxyBase+".http", "port"); err == nil {
		cfg.HTTPPort = gvariantToInt(port)
	}
	if host, err := m.query.Run("gsettings", "get", gsettingsProxyBase+".https", "host"); err == nil {
		cfg.HTTPSHost = gvariantUnquote(host)
	}
	if port, err := m.query.Run("gsettings", "get", gsettingsProxyBase+".https", "port"); err == nil {
		cfg.HTTPSPort = gvariantToInt(port)
	}
	if raw, err := m.query.Run("gsettings", "get", gsettingsProxyBase, "ignore-hosts"); err == nil {
		cfg.Bypass = gvariantArray(raw)
	}
	return cfg, nil
}

func gvariantToInt(v string) int {
	n := 0
	for _, r := range v {
		if r >= '0' && r <= '9' {
			n = n*10 + int(r-'0')
		}
	}
	return n
}

func (m *linuxManager) SetProxy(iface string, cfg ProxyConfig) error {
	_ = iface
	if !hasGsettings() {
		return newError(CodeUnsupported, "gsettings not found; cannot set a desktop proxy")
	}
	if cfg.Mode != "none" && cfg.Mode != "manual" {
		return newError(CodeInvalidArgument, "unknown proxy mode %q", cfg.Mode)
	}
	if cfg.Mode == "none" {
		return errNil(m.mutate.Run("gsettings", "set", gsettingsProxyBase, "mode", "'none'"))
	}
	if cfg.HTTPHost == "" || cfg.HTTPPort <= 0 {
		return newError(CodeInvalidArgument, "manual proxy needs http_host and http_port")
	}
	if !validProxyHost(cfg.HTTPHost) || (cfg.HTTPSHost != "" && !validProxyHost(cfg.HTTPSHost)) {
		return newError(CodeInvalidArgument, "proxy host is not an IP or hostname")
	}
	httpsHost, httpsPort := cfg.HTTPSHost, cfg.HTTPSPort
	if httpsHost == "" || httpsPort <= 0 {
		httpsHost, httpsPort = cfg.HTTPHost, cfg.HTTPPort
	}
	steps := [][]string{
		{"gsettings", "set", gsettingsProxyBase + ".http", "host", "'" + cfg.HTTPHost + "'"},
		{"gsettings", "set", gsettingsProxyBase + ".http", "port", itoa(cfg.HTTPPort)},
		{"gsettings", "set", gsettingsProxyBase + ".https", "host", "'" + httpsHost + "'"},
		{"gsettings", "set", gsettingsProxyBase + ".https", "port", itoa(httpsPort)},
		{"gsettings", "set", gsettingsProxyBase, "ignore-hosts", gvariantArrayString(cfg.Bypass)},
		{"gsettings", "set", gsettingsProxyBase, "mode", "'manual'"},
	}
	for _, step := range steps {
		if err := errNil(m.mutate.Run(step[0], step[1:]...)); err != nil {
			return err
		}
	}
	return nil
}

// HostsPath returns the Linux hosts file location.
func HostsPath() string { return "/etc/hosts" }
