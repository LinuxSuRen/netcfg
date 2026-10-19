package netconf

import (
	"reflect"
	"strings"
	"testing"
)

func TestParsePSInterfaces(t *testing.T) {
	out := `[
  {
    "name": "Wi-Fi", "label": "Intel Wi-Fi 6", "type": "wifi",
    "hardware_addr": "A4-83-E7-11-22-33", "mtu": 1500, "up": true,
    "ips": [
      {"address": "192.168.1.5", "prefix": 24, "family": "ipv4", "dynamic": true},
      {"address": "fd00::5", "prefix": 64, "family": "ipv6", "dynamic": true}
    ],
    "dhcp": true, "gateway": "192.168.1.1",
    "dns": ["192.168.1.1"]
  }
]`
	ifaces, err := parsePSInterfaces(out)
	if err != nil {
		t.Fatal(err)
	}
	if len(ifaces) != 1 {
		t.Fatalf("len = %d", len(ifaces))
	}
	iface := ifaces[0]
	if iface.Name != "Wi-Fi" || iface.Type != "wifi" || !iface.Up || !iface.DHCP {
		t.Errorf("iface = %+v", iface)
	}
	if iface.HardwareAddr != "A4:83:E7:11:22:33" {
		t.Errorf("mac not normalized: %q", iface.HardwareAddr)
	}
	if len(iface.IPs) != 2 || iface.IPs[0].Netmask != "255.255.255.0" {
		t.Errorf("ips = %+v", iface.IPs)
	}
}

func TestParsePSInterfacesSingleObject(t *testing.T) {
	out := `{"name":"Ethernet","label":"USB LAN","type":"ethernet","hardware_addr":"11-22-33","mtu":1500,"up":true,"ips":[{"address":"10.0.0.5","prefix":24,"family":"ipv4","dynamic":false}],"dhcp":false,"gateway":"10.0.0.1","dns":[]}`
	ifaces, err := parsePSInterfaces(out)
	if err != nil {
		t.Fatal(err)
	}
	if len(ifaces) != 1 || ifaces[0].Name != "Ethernet" || ifaces[0].DHCP {
		t.Errorf("ifaces = %+v", ifaces)
	}
}

func TestParseWlanNetworksEnglish(t *testing.T) {
	out := `There are 3 networks currently visible to the system.

SSID 1 : HomeNet
Network type            : Infrastructure
Authentication          : WPA2-Personal
Encryption              : CCMP
BSSID 1                 : aa:bb:cc:dd:ee:01
Signal                  : 86%
Channel                 : 6

SSID 2 : Neighbor 5G
Network type            : Infrastructure
Authentication          : WPA3-Personal
Encryption              : GCMP-256
BSSID 1                 : aa:bb:cc:dd:ee:02
Signal                  : 42%
Channel                 : 36

SSID 3 :
Network type            : Infrastructure
Authentication          : Open
Encryption              : None
BSSID 1                 : aa:bb:cc:dd:ee:03
Signal                  : 71%
Channel                 : 11
`
	nets := parseWlanNetworks(out, "Neighbor 5G")
	if len(nets) != 2 {
		t.Fatalf("len = %d: %+v", len(nets), nets)
	}
	if nets[0].SSID != "HomeNet" || nets[0].Signal != 86 || !strings.Contains(nets[0].Security, "WPA2") {
		t.Errorf("net0 = %+v", nets[0])
	}
	if nets[1].SSID != "Neighbor 5G" || !nets[1].Current {
		t.Errorf("net1 = %+v", nets[1])
	}
	// the trailing "SSID 3 :" block with an empty name must be dropped
	if nets[len(nets)-1].SSID == "" {
		t.Error("empty SSID leaked")
	}
}

func TestParseWlanNetworksChinese(t *testing.T) {
	out := `系统上有 1 个网络可用。

SSID 1 : 办公网
网络类型                : 结构
身份验证                : WPA2-个人
加密                    : CCMP
BSSID 1                 : aa:bb:cc:dd:ee:01
信号                    : 99%
通道                    : 6
`
	nets := parseWlanNetworks(out, "")
	if len(nets) != 1 {
		t.Fatalf("len = %d", len(nets))
	}
	if nets[0].SSID != "办公网" || nets[0].Signal != 99 || nets[0].BSSID != "aa:bb:cc:dd:ee:01" {
		t.Errorf("net = %+v", nets[0])
	}
}

func TestParseWlanCurrentSSID(t *testing.T) {
	out := `系统上有 1 个接口

    名称                   : Wi-Fi
    描述                   : Intel(R) Wi-Fi 6
    SSID                   : 办公网
    BSSID                  : aa:bb:cc:dd:ee:01
    状态                   : 已连接
`
	if got := parseWlanCurrentSSID(out); got != "办公网" {
		t.Errorf("ssid = %q", got)
	}
	if got := parseWlanCurrentSSID("    状态    : 已断开连接\n"); got != "" {
		t.Errorf("disconnected ssid = %q", got)
	}
}

func TestParsePSProxy(t *testing.T) {
	off, err := parsePSProxy(`{"ProxyEnable":0,"ProxyServer":"","ProxyOverride":""}`)
	if err != nil || off.Mode != "none" {
		t.Errorf("off = %+v err=%v", off, err)
	}
	on, err := parsePSProxy(`{"ProxyEnable":1,"ProxyServer":"proxy.example.com:8080","ProxyOverride":"*.local;169.254/16;<local>"}`)
	if err != nil {
		t.Fatal(err)
	}
	if on.Mode != "manual" || on.HTTPHost != "proxy.example.com" || on.HTTPPort != 8080 {
		t.Errorf("on = %+v", on)
	}
	if on.HTTPSHost != "proxy.example.com" || on.HTTPSPort != 8080 {
		t.Errorf("https mirror missing: %+v", on)
	}
	if !reflect.DeepEqual(on.Bypass, []string{"*.local", "169.254/16", "<local>"}) {
		t.Errorf("bypass = %v", on.Bypass)
	}
	split, err := parsePSProxy(`{"ProxyEnable":1,"ProxyServer":"http=p:1;https=q:2","ProxyOverride":""}`)
	if err != nil {
		t.Fatal(err)
	}
	if split.HTTPHost != "p" || split.HTTPPort != 1 || split.HTTPSHost != "q" || split.HTTPSPort != 2 {
		t.Errorf("split = %+v", split)
	}
}

func TestBuildProxyServer(t *testing.T) {
	same := buildProxyServer(ProxyConfig{HTTPHost: "p", HTTPPort: 1, HTTPSHost: "p", HTTPSPort: 1})
	if same != "p:1" {
		t.Errorf("same = %q", same)
	}
	diff := buildProxyServer(ProxyConfig{HTTPHost: "p", HTTPPort: 1, HTTPSHost: "q", HTTPSPort: 2})
	if diff != "http=p:1;https=q:2" {
		t.Errorf("diff = %q", diff)
	}
}

func TestWLANProfileXML(t *testing.T) {
	secured, err := wlanProfileXML("Home&Net", "pa<ss>")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(secured, "WPA2PSK") || !strings.Contains(secured, "Home&amp;Net") ||
		!strings.Contains(secured, "pa&lt;ss&gt;") {
		t.Errorf("secured profile malformed:\n%s", secured)
	}
	open, err := wlanProfileXML("cafe", "")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(open, "<authentication>open</authentication>") ||
		strings.Contains(open, "sharedKey") {
		t.Errorf("open profile malformed:\n%s", open)
	}
	if _, err := wlanProfileXML("", "x"); err == nil {
		t.Error("empty ssid should fail")
	}
}
