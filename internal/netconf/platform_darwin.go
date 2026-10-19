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
	path := airportPath()
	if path == "" {
		return nil, newError(CodeUnsupported, "airport tool not found; scanning is unavailable on this macOS")
	}
	out, err := m.query.Run(path, "-s")
	if err != nil {
		return nil, err
	}
	return parseAirportScan(out), nil
}

func (m *darwinManager) ConnectWifi(iface, ssid, password string) error {
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
