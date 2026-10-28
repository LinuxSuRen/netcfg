//go:build darwin

package netconf

import (
	"net"
	"os"
	"strconv"
	"strings"
)

// darwinManager implements Manager on top of networksetup(1), ifconfig(1)
// and the Apple80211 airport tool. Mutations go through the mutate runner so
// --dry-run can record them while discovery keeps using real queries.
type darwinManager struct {
	query  Runner
	mutate Runner
	// requestLocation is injectable for tests; nil means the real
	// compile-and-launch helper runs.
	requestLocation func() bool
}

// NewManager returns the macOS backend.
func NewManager() Manager { return &darwinManager{query: ExecRunner{}, mutate: ExecRunner{}} }

// NewDryManager returns the macOS backend recording mutations instead of
// running them.
func NewDryManager() Manager { return &darwinManager{query: ExecRunner{}, mutate: &DryRunner{}} }

// DryCommands returns the recorded mutations (used by the CLI dry run).
func (m *darwinManager) DryCommands() []string {
	if d, ok := m.mutate.(*DryRunner); ok {
		return d.Commands
	}
	return nil
}

func (m *darwinManager) Capabilities() Capabilities {
	return Capabilities{SetIP: true, SetDNS: true, WifiScan: true, WifiJoin: true, Proxy: true, Hosts: true}
}

func (m *darwinManager) services() ([]string, error) {
	out, err := m.query.Run("networksetup", "-listallnetworkservices")
	if err != nil {
		return nil, err
	}
	return parseDarwinServices(out), nil
}

func (m *darwinManager) serviceOrder() ([]darwinServiceInfo, error) {
	out, err := m.query.Run("networksetup", "-listnetworkserviceorder")
	if err != nil {
		return nil, err
	}
	return parseDarwinServiceOrder(out), nil
}

func (m *darwinManager) Interfaces() ([]Interface, error) {
	services, err := m.services()
	if err != nil {
		return nil, err
	}
	order, _ := m.serviceOrder()
	deviceOf := map[string]darwinServiceInfo{}
	for _, info := range order {
		deviceOf[info.Service] = info
	}
	var links map[string]darwinLink
	if out, err := m.query.Run("ifconfig"); err == nil {
		links = parseDarwinIfconfig(out)
	}

	var result []Interface
	for _, svc := range services {
		iface := Interface{Name: svc, Label: svc}
		if info, ok := deviceOf[svc]; ok {
			iface.Type = darwinClassify(info.Port, info.Device)
			if link, ok := links[info.Device]; ok {
				iface.Up, iface.MTU, iface.HardwareAddr = link.Up, link.MTU, link.Ether
			}
		} else {
			iface.Type = "other"
		}
		if out, err := m.query.Run("networksetup", "-getinfo", svc); err == nil {
			dhcp, ipv4s, masks, gw, ipv6s := parseDarwinGetInfo(out)
			iface.DHCP = dhcp
			iface.Gateway = gw
			for i, ip := range ipv4s {
				mask := ""
				if i < len(masks) {
					mask = masks[i]
				}
				iface.IPs = append(iface.IPs, IPInfo{Address: ip, Netmask: mask, Family: "ipv4", Dynamic: dhcp})
			}
			for _, ip := range ipv6s {
				iface.IPs = append(iface.IPs, IPInfo{Address: ip, Family: "ipv6", Dynamic: dhcp})
			}
		}
		if out, err := m.query.Run("networksetup", "-getdnsservers", svc); err == nil {
			iface.DNS = parseDarwinDNS(out)
		}
		if iface.Type == "wifi" {
			if out, err := m.query.Run("networksetup", "-getairportnetwork", svc); err == nil {
				iface.SSID = parseDarwinAirportNetwork(out)
			}
			// getairportnetwork is redacted without Location access; wdutil
			// still answers under sudo.
			if iface.SSID == "" {
				if net := m.wdutilNetwork(); net != nil {
					iface.SSID = net.SSID
				}
			}
		}
		result = append(result, iface)
	}
	return result, nil
}

// hwAddr note: hardware addresses come from the ifconfig block of the
// service's device; see Interfaces().

func (m *darwinManager) SetIP(name string, cfg IPConfig) error {
	if err := m.requireService(name); err != nil {
		return err
	}
	if cfg.DHCP {
		_, err := m.mutate.Run("networksetup", "-setdhcp", name)
		return err
	}
	ip := net.ParseIP(cfg.IP)
	if ip == nil || ip.To4() == nil {
		return newError(CodeInvalidArgument, "%q is not an IPv4 address", cfg.IP)
	}
	if _, ok := maskToPrefixLen(cfg.Netmask); !ok {
		return newError(CodeInvalidArgument, "%q is not a valid netmask", cfg.Netmask)
	}
	if cfg.Gateway != "" {
		if gw := net.ParseIP(cfg.Gateway); gw == nil || gw.To4() == nil {
			return newError(CodeInvalidArgument, "%q is not an IPv4 address", cfg.Gateway)
		}
		return errNil(m.mutate.Run("networksetup", "-setmanual", name, cfg.IP, cfg.Netmask, cfg.Gateway))
	}
	return errNil(m.mutate.Run("networksetup", "-setmanual", name, cfg.IP, cfg.Netmask))
}

func (m *darwinManager) SetDNS(name string, cfg DNSConfig) error {
	if err := m.requireService(name); err != nil {
		return err
	}
	if cfg.Auto {
		_, err := m.mutate.Run("networksetup", "-setdnsservers", name, "Empty")
		return err
	}
	for _, s := range cfg.Servers {
		if net.ParseIP(s) == nil {
			return newError(CodeInvalidArgument, "%q is not an IP address", s)
		}
	}
	return errNil(m.mutate.Run("networksetup", append([]string{"-setdnsservers", name}, cfg.Servers...)...))
}

func (m *darwinManager) requireService(name string) error {
	services, err := m.services()
	if err != nil {
		return err
	}
	for _, svc := range services {
		if strings.EqualFold(svc, name) {
			return nil
		}
	}
	return ifaceNotFound(name)
}

// airportPath locates the Apple80211 scan tool.
func airportPath() string {
	candidates := []string{
		"/System/Library/PrivateFrameworks/Apple80211.framework/Versions/Current/Resources/airport",
		"/System/Library/PrivateFrameworks/Apple80211.framework/Resources/airport",
	}
	for _, p := range candidates {
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	return ""
}

func (m *darwinManager) ScanWifi() ([]WifiNetwork, error) {
	// The airport CLI is deprecated and on macOS 15+ returns no rows even
	// for root; try it first for older systems, then fall back to
	// system_profiler which still lists Wi-Fi networks.
	if path := airportPath(); path != "" {
		if out, err := m.query.Run(path, "-s"); err == nil {
			if nets := parseAirportScan(out); len(nets) > 0 {
				return nets, nil
			}
		}
	}
	out, err := m.query.Run("system_profiler", "SPAirPortDataType")
	if err != nil {
		return nil, err
	}
	_, nets, redacted := parseSystemProfilerAirport(out)
	if len(nets) > 0 {
		return nets, nil
	}
	// Location Services hides every SSID from this process; wdutil (under
	// sudo) still reports the current network — return that instead of an
	// outright error so the UI stays usable.
	if net := m.wdutilNetwork(); net != nil {
		return []WifiNetwork{*net}, nil
	}
	if redacted > 0 {
		// SSIDs are visible but Location Services hides them from this
		// process. netcfg itself triggers the authorization prompt (the
		// grant is recorded against the terminal app by macOS); once the
		// user allows it, a re-scan lists everything.
		if m.triggerLocationAuth() {
			return nil, newError(CodePermissionDenied,
				"a Location authorization prompt has been triggered; click Allow in the dialog, then scan again")
		}
		return nil, newError(CodePermissionDenied,
			"Wi-Fi SSIDs are redacted by Location Services; grant the terminal app Location access in System Settings > Privacy & Security > Location Services and try again")
	}
	return nil, newError(CodePermissionDenied,
		"no Wi-Fi networks could be listed; scanning needs administrator privileges and terminal Location access, restart netcfg with sudo and try again")
}

// triggerLocationAuth runs the injectable test hook or the real helper.
func (m *darwinManager) triggerLocationAuth() bool {
	if m.requestLocation != nil {
		return m.requestLocation()
	}
	return requestLocationAuthOnce()
}

// wdutilNetwork reads the current network from wdutil, which reports the
// SSID under sudo even when Location Services redacts everything else.
func (m *darwinManager) wdutilNetwork() *WifiNetwork {
	out, err := m.query.Run("wdutil", "info")
	if err != nil {
		return nil
	}
	return parseWdutilInfo(out)
}

func (m *darwinManager) ConnectWifi(iface, ssid, password string) error {
	if !validSSID(ssid) {
		return newError(CodeInvalidArgument, "invalid SSID %q", ssid)
	}
	if iface == "" {
		service, err := m.defaultWifiService()
		if err != nil {
			return err
		}
		iface = service
	}
	if err := m.requireService(iface); err != nil {
		return err
	}
	args := []string{"-setairportnetwork", iface, ssid}
	if password != "" {
		args = append(args, password)
	}
	_, err := m.mutate.Run("networksetup", args...)
	return err
}

// StartWifiAP is not implemented on macOS: Internet Sharing has no stable
// CLI to drive (it is configured through the Sharing preference pane or
// private frameworks). The capability reports false accordingly.
func (m *darwinManager) StartWifiAP(iface, ssid, password string, opts WifiAPOptions) error {
	_ = iface
	_ = ssid
	_ = password
	_ = opts
	return newError(CodeUnsupported, "starting a hotspot is not supported on macOS; use System Settings > Sharing > Internet Sharing")
}

func (m *darwinManager) StopWifiAP(opts WifiAPOptions) error {
	_ = opts
	return newError(CodeUnsupported, "stopping a hotspot is not supported on macOS; use System Settings > Sharing > Internet Sharing")
}

func (m *darwinManager) defaultWifiService() (string, error) {
	ifaces, err := m.Interfaces()
	if err != nil {
		return "", err
	}
	for _, iface := range ifaces {
		if iface.Type == "wifi" {
			return iface.Name, nil
		}
	}
	return "", newError(CodeNotFound, "no Wi-Fi service found")
}

func (m *darwinManager) Proxy(iface string) (ProxyConfig, error) {
	svc, err := m.proxyService(iface)
	if err != nil {
		return ProxyConfig{}, err
	}
	cfg := ProxyConfig{Mode: "none"}
	if out, err := m.query.Run("networksetup", "-getwebproxy", svc); err == nil {
		if on, host, port := parseDarwinProxy(out); on {
			cfg.Mode = "manual"
			cfg.HTTPHost, cfg.HTTPPort = host, port
		}
	}
	if out, err := m.query.Run("networksetup", "-getsecurewebproxy", svc); err == nil {
		if on, host, port := parseDarwinProxy(out); on {
			cfg.Mode = "manual"
			cfg.HTTPSHost, cfg.HTTPSPort = host, port
		}
	}
	if out, err := m.query.Run("networksetup", "-getproxybypassdomains", svc); err == nil {
		cfg.Bypass = parseDarwinBypass(out)
	}
	return cfg, nil
}

func (m *darwinManager) SetProxy(iface string, cfg ProxyConfig) error {
	svc, err := m.proxyService(iface)
	if err != nil {
		return err
	}
	if cfg.Mode == "none" {
		if err := errNil(m.mutate.Run("networksetup", "-setwebproxystate", svc, "off")); err != nil {
			return err
		}
		return errNil(m.mutate.Run("networksetup", "-setsecurewebproxystate", svc, "off"))
	}
	if cfg.Mode != "manual" {
		return newError(CodeInvalidArgument, "unknown proxy mode %q", cfg.Mode)
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
		{"-setwebproxy", svc, cfg.HTTPHost, strconv.Itoa(cfg.HTTPPort)},
		{"-setsecurewebproxy", svc, httpsHost, strconv.Itoa(httpsPort)},
	}
	if len(cfg.Bypass) > 0 {
		steps = append(steps, append([]string{"-setproxybypassdomains", svc}, cfg.Bypass...))
	} else {
		steps = append(steps, []string{"-setproxybypassdomains", svc, "Empty"})
	}
	for _, step := range steps {
		if err := errNil(m.mutate.Run("networksetup", step...)); err != nil {
			return err
		}
	}
	return nil
}

// proxyService resolves the network service a proxy operation targets.
func (m *darwinManager) proxyService(iface string) (string, error) {
	if iface != "" {
		return iface, m.requireService(iface)
	}
	if service, err := m.defaultWifiService(); err == nil {
		return service, nil
	}
	services, err := m.services()
	if err != nil {
		return "", err
	}
	if len(services) == 0 {
		return "", newError(CodeNotFound, "no network service found")
	}
	return services[0], nil
}

// HostsPath returns the macOS hosts file location.
func HostsPath() string { return "/etc/hosts" }
