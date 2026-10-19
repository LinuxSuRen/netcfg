package netconf

import (
	"reflect"
	"testing"
)

func TestSplitTerse(t *testing.T) {
	// nmcli escapes colons inside values as "\:"; separators stay bare.
	got := splitTerse(`My\:SSID:aa\:bb\:cc\:dd\:ee\:ff:6:72:WPA2:yes`)
	want := []string{"My:SSID", "aa:bb:cc:dd:ee:ff", "6", "72", "WPA2", "yes"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v want %v", got, want)
	}
}

func TestParseNMDeviceShow(t *testing.T) {
	out := `GENERAL.DEVICE:wlan0
GENERAL.TYPE:wifi
GENERAL.HWADDR:AA:BB:CC:DD:EE:FF
GENERAL.MTU:1500
GENERAL.STATE:100 (connected)
GENERAL.CONNECTION:HomeNet
IP4.ADDRESS[1]:192.168.1.5/24
IP4.GATEWAY:192.168.1.1
IP4.DNS[1]:192.168.1.1

GENERAL.DEVICE:eth0
GENERAL.TYPE:ethernet
GENERAL.HWADDR:11:22:33:44:55:66
GENERAL.MTU:1500
GENERAL.STATE:30 (disconnected)
GENERAL.CONNECTION:--

GENERAL.DEVICE:lo
GENERAL.TYPE:loopback
GENERAL.MTU:65536
GENERAL.STATE:10 (unmanaged)
GENERAL.CONNECTION:--

`
	devices := parseNMDeviceShow(out)
	if len(devices) != 3 {
		t.Fatalf("len = %d: %+v", len(devices), devices)
	}
	wifi := devices[0]
	if wifi.Device != "wlan0" || wifi.Type != "wifi" || wifi.Connection != "HomeNet" {
		t.Errorf("wifi = %+v", wifi)
	}
	if !reflect.DeepEqual(wifi.IP4s, []string{"192.168.1.5/24"}) || wifi.Gateway != "192.168.1.1" {
		t.Errorf("wifi addrs = %+v", wifi)
	}
	if !reflect.DeepEqual(wifi.DNS, []string{"192.168.1.1"}) {
		t.Errorf("dns = %v", wifi.DNS)
	}
	eth := devices[1]
	if eth.Connection != "" {
		t.Errorf("disconnected connection should be empty: %+v", eth)
	}
}

func TestParseNMConnectionMethods(t *testing.T) {
	out := `Wired connection 1:eth0:auto
HomeNet:wlan0:manual
lo:lo:disabled
`
	got := parseNMConnectionMethods(out)
	if !got["eth0"] || got["wlan0"] || got["lo"] {
		t.Errorf("got = %#v", got)
	}
}

func TestParseNMWifiList(t *testing.T) {
	out := `HomeNet:aa\:bb\:cc\:dd\:ee\:01:6:72:WPA2:yes
Neighbor:aa\:bb\:cc\:dd\:ee\:02:11:40:WPA1 WPA2:no
cafe:aa\:bb\:cc\:dd\:ee\:03:36:65::no
`
	nets := parseNMWifiList(out, true)
	if len(nets) != 3 {
		t.Fatalf("len = %d: %+v", len(nets), nets)
	}
	if !nets[0].Current || nets[0].SSID != "HomeNet" || nets[0].Signal != 72 {
		t.Errorf("net0 = %+v", nets[0])
	}
	if nets[2].Security != "" {
		t.Errorf("open security should be empty: %+v", nets[2])
	}
}

func TestGvariantHelpers(t *testing.T) {
	if got := gvariantUnquote("'manual'"); got != "manual" {
		t.Errorf("unquote = %q", got)
	}
	got := gvariantArray(`['localhost', '127.0.0.0/8']`)
	if !reflect.DeepEqual(got, []string{"localhost", "127.0.0.0/8"}) {
		t.Errorf("array = %v", got)
	}
	if back := gvariantArrayString(got); back != `['localhost', '127.0.0.0/8']` {
		t.Errorf("roundtrip = %q", back)
	}
	if empty := gvariantArrayString(nil); empty != "@as []" {
		t.Errorf("empty = %q", empty)
	}
}

func TestParseIPAddrAndRoute(t *testing.T) {
	out := `[{"ifname":"eth0","flags":["BROADCAST","MULTICAST","UP"],"mtu":1500,"operstate":"up",
  "address":"11:22:33:44:55:66",
  "addr_info":[{"family":"inet","local":"192.168.1.5","prefixlen":24},
               {"family":"inet6","local":"fe80::1","prefixlen":64}]},
 {"ifname":"lo","flags":["LOOPBACK","UP"],"mtu":65536,"operstate":"unknown",
  "addr_info":[{"family":"inet","local":"127.0.0.1","prefixlen":8}]}]`
	ifaces, err := parseIPAddr(out)
	if err != nil {
		t.Fatal(err)
	}
	if len(ifaces) != 2 {
		t.Fatalf("len = %d", len(ifaces))
	}
	eth := ifaces[0]
	if !eth.Up || eth.Type != "ethernet" || eth.HardwareAddr != "11:22:33:44:55:66" {
		t.Errorf("eth = %+v", eth)
	}
	if len(eth.IPs) != 1 || eth.IPs[0].Netmask != "255.255.255.0" || eth.IPs[0].Address != "192.168.1.5" {
		t.Errorf("eth ips = %+v (link-local must be dropped)", eth.IPs)
	}
	if ifaces[1].Type != "loopback" || len(ifaces[1].IPs) != 0 {
		t.Errorf("lo = %+v", ifaces[1])
	}
	routes := parseIPRoute(`[{"dst":"default","dev":"eth0","gateway":"192.168.1.1"}]`)
	if routes["eth0"] != "192.168.1.1" {
		t.Errorf("routes = %v", routes)
	}
}

func TestParseResolvConf(t *testing.T) {
	content := `# comment
nameserver 192.168.1.1
nameserver 8.8.8.8
search example.com
`
	got := parseResolvConf(content)
	if !reflect.DeepEqual(got, []string{"192.168.1.1", "8.8.8.8"}) {
		t.Errorf("got = %v", got)
	}
}

func TestSplitCIDR(t *testing.T) {
	ip, prefix, ok := splitCIDR("192.168.1.5/24")
	if !ok || ip != "192.168.1.5" || prefix != 24 {
		t.Errorf("got %q %d %v", ip, prefix, ok)
	}
	if _, _, ok := splitCIDR("192.168.1.5"); ok {
		t.Error("missing prefix should fail")
	}
}
