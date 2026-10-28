//go:build windows

package netconf

import (
	"net"
	"os"
	"path/filepath"
	"strings"
)

// windowsManager implements Manager on top of PowerShell (Get-Net* read),
// netsh (writes, WLAN) and the WinINET per-user proxy registry keys. netsh
// subcommand keywords are English regardless of UI locale; only output is
// localized, which is why reads go through PowerShell JSON.
type windowsManager struct {
	query  Runner
	mutate Runner
}

// NewManager returns the Windows backend.
func NewManager() Manager { return &windowsManager{query: ExecRunner{}, mutate: ExecRunner{}} }

// NewDryManager returns the Windows backend recording mutations instead of
// running them.
func NewDryManager() Manager {
	return &windowsManager{query: ExecRunner{}, mutate: &DryRunner{}}
}

// DryCommands returns the recorded mutations (used by the CLI dry run).
func (m *windowsManager) DryCommands() []string {
	if d, ok := m.mutate.(*DryRunner); ok {
		return d.Commands
	}
	return nil
}

func (m *windowsManager) Capabilities() Capabilities {
	return Capabilities{SetIP: true, SetDNS: true, WifiScan: true, WifiJoin: true, Proxy: true, Hosts: true}
}

// psInterfacesScript builds one PowerShell invocation returning all
// adapters with addresses, routes and DNS as JSON.
const psInterfacesScript = `$ErrorActionPreference='SilentlyContinue'
$routes=@{}
Get-NetRoute -DestinationPrefix '0.0.0.0/0' | ForEach-Object { if (-not $routes.ContainsKey($_.InterfaceAlias)) { $routes[$_.InterfaceAlias]=$_.NextHop } }
$dnss=@{}
Get-DnsClientServerAddress -AddressFamily IPv4 | ForEach-Object { if ($_.ServerAddresses) { $dnss[$_.InterfaceAlias]=@($_.ServerAddresses) } }
$out=@()
Get-NetAdapter | ForEach-Object {
  $a=$_
  $ips=@(Get-NetIPAddress -InterfaceIndex $a.ifIndex | Where-Object { $_.IPAddress -ne '127.0.0.1' -and $_.IPAddress -notlike 'fe80*' } | ForEach-Object {
    $fam='ipv6'; if ($_.AddressFamily -eq 2) { $fam='ipv4' }
    @{ address=$_.IPAddress; prefix=$_.PrefixLength; family=$fam; dynamic=($_.PrefixOrigin -eq 'Dhcp') }
  })
  $dhcp=$false
  $ips | ForEach-Object { if ($_['dynamic']) { $dhcp=$true } }
  $type='ethernet'; if ($a.PhysicalMediaType -eq 'Native 802.11') { $type='wifi' }
  $gw=''
  if ($routes.ContainsKey($a.Name)) { $gw=$routes[$a.Name] }
  $d=@()
  if ($dnss.ContainsKey($a.Name)) { $d=$dnss[$a.Name] }
  $out += @{ name=$a.Name; label=$a.InterfaceDescription; type=$type; hardware_addr=$a.MacAddress; mtu=$a.MtuSize; up=($a.Status -eq 'Up'); ips=$ips; dhcp=$dhcp; gateway=$gw; dns=$d }
}
ConvertTo-Json -InputObject @($out) -Depth 6`

func (m *windowsManager) Interfaces() ([]Interface, error) {
	out, err := m.query.Run("powershell", "-NoProfile", "-NonInteractive", "-Command", psInterfacesScript)
	if err != nil {
		return nil, err
	}
	ifaces, err := parsePSInterfaces(out)
	if err != nil {
		return nil, err
	}
	current := m.currentSSID()
	for i := range ifaces {
		if ifaces[i].Type == "wifi" && current != "" {
			ifaces[i].SSID = current
		}
	}
	return ifaces, nil
}

// currentSSID returns the SSID of the connected WLAN interface.
func (m *windowsManager) currentSSID() string {
	out, err := m.query.Run("netsh", "wlan", "show", "interfaces")
	if err != nil {
		return ""
	}
	return parseWlanCurrentSSID(out)
}

func (m *windowsManager) requireInterface(name string) error {
	ifaces, err := m.Interfaces()
	if err != nil {
		return err
	}
	for _, iface := range ifaces {
		if strings.EqualFold(iface.Name, name) {
			return nil
		}
	}
	return ifaceNotFound(name)
}

func (m *windowsManager) SetIP(name string, cfg IPConfig) error {
	if err := m.requireInterface(name); err != nil {
		return err
	}
	if cfg.DHCP {
		return errNil(m.mutate.Run("netsh", "interface", "ipv4", "set", "address",
			"name="+name, "source=dhcp"))
	}
	ip := net.ParseIP(cfg.IP)
	if ip == nil || ip.To4() == nil {
		return newError(CodeInvalidArgument, "%q is not an IPv4 address", cfg.IP)
	}
	prefix, ok := maskToPrefixLen(cfg.Netmask)
	if !ok {
		return newError(CodeInvalidArgument, "%q is not a valid netmask", cfg.Netmask)
	}
	_ = prefix // netsh takes the netmask itself
	args := []string{"interface", "ipv4", "set", "address", "name=" + name,
		"source=static", "address=" + cfg.IP, "mask=" + cfg.Netmask}
	if cfg.Gateway != "" {
		if gw := net.ParseIP(cfg.Gateway); gw == nil || gw.To4() == nil {
			return newError(CodeInvalidArgument, "%q is not an IPv4 address", cfg.Gateway)
		}
		args = append(args, "gateway="+cfg.Gateway)
	}
	return errNil(m.mutate.Run("netsh", args...))
}

func (m *windowsManager) SetDNS(name string, cfg DNSConfig) error {
	if err := m.requireInterface(name); err != nil {
		return err
	}
	if cfg.Auto {
		return errNil(m.mutate.Run("netsh", "interface", "ipv4", "set", "dnsservers",
			"name="+name, "source=dhcp"))
	}
	if len(cfg.Servers) == 0 {
		return newError(CodeInvalidArgument, "dns servers list is empty (use auto to clear)")
	}
	for _, s := range cfg.Servers {
		if net.ParseIP(s) == nil {
			return newError(CodeInvalidArgument, "%q is not an IP address", s)
		}
	}
	if err := errNil(m.mutate.Run("netsh", "interface", "ipv4", "set", "dnsservers",
		"name="+name, "source=static", "address="+cfg.Servers[0], "validate=no")); err != nil {
		return err
	}
	for i, s := range cfg.Servers[1:] {
		if err := errNil(m.mutate.Run("netsh", "interface", "ipv4", "add", "dns",
			"name="+name, "address="+s, "index="+itoa(i+2), "validate=no")); err != nil {
			return err
		}
	}
	return nil
}

func (m *windowsManager) ScanWifi() ([]WifiNetwork, error) {
	current := m.currentSSID()
	out, err := m.query.Run("netsh", "wlan", "show", "networks", "mode=bssid")
	if err != nil {
		return nil, err
	}
	return parseWlanNetworks(out, current), nil
}

func (m *windowsManager) ConnectWifi(iface, ssid, password string) error {
	if !validSSID(ssid) {
		return newError(CodeInvalidArgument, "invalid SSID %q", ssid)
	}
	profile, err := wlanProfileXML(ssid, password)
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp("", "netcfg-wlan-*.xml")
	if err != nil {
		return newError(CodeCommandFailed, "create temp profile: %v", err)
	}
	path := tmp.Name()
	defer os.Remove(path)
	if _, err := tmp.WriteString(profile); err != nil {
		tmp.Close()
		return newError(CodeCommandFailed, "write temp profile: %v", err)
	}
	tmp.Close()
	if _, err := m.mutate.Run("netsh", "wlan", "add", "profile",
		"filename="+path, "user=all"); err != nil {
		return err
	}
	args := []string{"wlan", "connect", "name=" + ssid}
	if iface != "" {
		args = append(args, "interface="+iface)
	}
	_, err = m.mutate.Run("netsh", args...)
	return err
}

// StartWifiAP is not implemented on Windows: the Mobile Hotspot feature is
// driven through WinRT (NetworkOperatorTetheringManager), not netsh, and is
// out of scope for now. The capability reports false accordingly.
func (m *windowsManager) StartWifiAP(iface, ssid, password string, opts WifiAPOptions) error {
	_ = iface
	_ = ssid
	_ = password
	_ = opts
	return newError(CodeUnsupported, "starting a hotspot is not supported on Windows yet; use Settings > Network & internet > Mobile hotspot")
}

func (m *windowsManager) StopWifiAP(opts WifiAPOptions) error {
	_ = opts
	return newError(CodeUnsupported, "stopping a hotspot is not supported on Windows yet; use Settings > Network & internet > Mobile hotspot")
}

const proxyRegPath = `HKCU\Software\Microsoft\Windows\CurrentVersion\Internet Settings`

func (m *windowsManager) Proxy(iface string) (ProxyConfig, error) {
	_ = iface // the WinINET proxy is per-user and machine-global
	out, err := m.query.Run("powershell", "-NoProfile", "-NonInteractive", "-Command",
		`Get-ItemProperty '`+proxyRegPath+`' | Select-Object ProxyEnable, ProxyServer, ProxyOverride, AutoConfigURL | ConvertTo-Json`)
	if err != nil {
		return ProxyConfig{}, err
	}
	return parsePSProxy(out)
}

func (m *windowsManager) SetProxy(iface string, cfg ProxyConfig) error {
	_ = iface
	if cfg.Mode != "none" && cfg.Mode != "manual" {
		return newError(CodeInvalidArgument, "unknown proxy mode %q", cfg.Mode)
	}
	if cfg.Mode == "none" {
		return errNil(m.mutate.Run("reg", "add", proxyRegPath, "/v", "ProxyEnable",
			"/t", "REG_DWORD", "/d", "0", "/f"))
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
	full := ProxyConfig{HTTPHost: cfg.HTTPHost, HTTPPort: cfg.HTTPPort,
		HTTPSHost: httpsHost, HTTPSPort: httpsPort, Bypass: cfg.Bypass}
	steps := [][]string{
		{"reg", "add", proxyRegPath, "/v", "ProxyEnable", "/t", "REG_DWORD", "/d", "1", "/f"},
		{"reg", "add", proxyRegPath, "/v", "ProxyServer", "/t", "REG_SZ", "/d", buildProxyServer(full), "/f"},
	}
	override := "<local>"
	if len(cfg.Bypass) > 0 {
		override = strings.Join(append(append([]string{}, cfg.Bypass...), "<local>"), ";")
	}
	steps = append(steps, []string{"reg", "add", proxyRegPath, "/v", "ProxyOverride",
		"/t", "REG_SZ", "/d", override, "/f"})
	for _, step := range steps {
		if err := errNil(m.mutate.Run(step[0], step[1:]...)); err != nil {
			return err
		}
	}
	return nil
}

// HostsPath returns the Windows hosts file location.
func HostsPath() string {
	root := os.Getenv("SystemRoot")
	if root == "" {
		root = `C:\Windows`
	}
	return filepath.Join(root, `System32\drivers\etc\hosts`)
}
