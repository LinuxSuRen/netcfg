package netconf

import (
	"reflect"
	"testing"
)

func TestParseDarwinServices(t *testing.T) {
	out := `An asterisk (*) denotes that a network service is disabled.
Wi-Fi
Ethernet
*Thunderbolt Bridge
`
	got := parseDarwinServices(out)
	want := []string{"Wi-Fi", "Ethernet", "Thunderbolt Bridge"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("services = %v, want %v", got, want)
	}
}

func TestParseDarwinServiceOrder(t *testing.T) {
	out := `Network services (2, least to most priority)
(1) Wi-Fi
(Hardware Port: Wi-Fi, Device: en0, Ethernet Address: aa:bb:cc:dd:ee:ff)

(2) Ethernet
(Hardware Port: Ethernet, Device: en5, Ethernet Address: 11:22:33:44:55:66)
`
	got := parseDarwinServiceOrder(out)
	if len(got) != 2 {
		t.Fatalf("len = %d, want 2: %+v", len(got), got)
	}
	if got[0].Service != "Wi-Fi" || got[0].Device != "en0" {
		t.Errorf("entry0 = %+v", got[0])
	}
	if got[1].Service != "Ethernet" || got[1].Device != "en5" {
		t.Errorf("entry1 = %+v", got[1])
	}
}

func TestParseDarwinGetInfoDHCP(t *testing.T) {
	out := `DHCP Configuration
IP address: 192.168.1.5
Subnet mask: 255.255.255.0
Router: 192.168.1.1
Wi-Fi ID: aa:bb:cc:dd:ee:ff
SSID: HomeNet
IPv6: Automatic
IPv6 IP address: fe80::1%en0
`
	dhcp, ipv4s, masks, gw, ipv6s := parseDarwinGetInfo(out)
	if !dhcp {
		t.Error("dhcp = false, want true")
	}
	if !reflect.DeepEqual(ipv4s, []string{"192.168.1.5"}) {
		t.Errorf("ipv4s = %v", ipv4s)
	}
	if !reflect.DeepEqual(masks, []string{"255.255.255.0"}) {
		t.Errorf("masks = %v", masks)
	}
	if gw != "192.168.1.1" {
		t.Errorf("gateway = %q", gw)
	}
	if len(ipv6s) != 0 {
		t.Errorf("ipv6s should drop link-local, got %v", ipv6s)
	}
}

func TestParseDarwinGetInfoManualLegacy(t *testing.T) {
	out := `Manual Configuration
IP Addresses (IPv4): 10.0.0.5
Subnet Masks (IPv4): 255.255.0.0
Router (IPv4): 10.0.0.1
Client ID:
IPv6: Off
`
	dhcp, ipv4s, masks, gw, _ := parseDarwinGetInfo(out)
	if dhcp {
		t.Error("dhcp = true, want false")
	}
	if !reflect.DeepEqual(ipv4s, []string{"10.0.0.5"}) {
		t.Errorf("ipv4s = %v", ipv4s)
	}
	if !reflect.DeepEqual(masks, []string{"255.255.0.0"}) {
		t.Errorf("masks = %v", masks)
	}
	if gw != "10.0.0.1" {
		t.Errorf("gateway = %q", gw)
	}
}

func TestParseDarwinDNS(t *testing.T) {
	empty := parseDarwinDNS("There aren't any DNS Servers set on Wi-Fi. You may set them manually.")
	if len(empty) != 0 {
		t.Errorf("empty = %v", empty)
	}
	got := parseDarwinDNS("192.168.1.1\n8.8.8.8")
	if !reflect.DeepEqual(got, []string{"192.168.1.1", "8.8.8.8"}) {
		t.Errorf("got = %v", got)
	}
}

func TestParseDarwinIfconfig(t *testing.T) {
	out := `lo0: flags=8049<LOOPBACK,RUNNING,MULTICAST> mtu 16384
	inet 127.0.0.1 netmask 0xff000000
en0: flags=8863<UP,BROADCAST,SMART,RUNNING,SIMPLEX,MULTICAST> mtu 1500
	ether a4:83:e7:11:22:33
	inet 192.168.1.5 netmask 0xffffff00 broadcast 192.168.1.255
en5: flags=8862<BROADCAST,SMART,RUNNING,SIMPLEX,MULTICAST> mtu 1500
	ether 11:22:33:44:55:66
`
	links := parseDarwinIfconfig(out)
	if v, ok := links["lo0"]; !ok || v.Up || v.MTU != 16384 {
		t.Errorf("lo0 = %+v ok=%v", v, ok)
	}
	en0 := links["en0"]
	if !en0.Up || en0.MTU != 1500 || en0.Ether != "a4:83:e7:11:22:33" {
		t.Errorf("en0 = %+v", en0)
	}
	if en5, ok := links["en5"]; !ok || en5.Up {
		t.Errorf("en5 = %+v ok=%v", en5, ok)
	}
}

func TestParseAirportScan(t *testing.T) {
	out := `                            SSID BSSID             RSSI CHANNEL HT CC SECURITY (auth/unicast/group)
              HomeNetWithSpace a4:2b:8c:11:22:33 -59  6,+1   Y  US WPA2(PSK/AES, 5/6)
                       Neighbor a4:2b:8c:44:55:66 -80  11     N  -- WPA(PSK/TKIP, 3/2)
                             cafe aa:11:bb:22:cc:33 -50  36,+1  Y  CN --
`
	nets := parseAirportScan(out)
	if len(nets) != 3 {
		t.Fatalf("len = %d: %+v", len(nets), nets)
	}
	first := nets[0]
	if first.SSID != "HomeNetWithSpace" || first.BSSID != "a4:2b:8c:11:22:33" {
		t.Errorf("first = %+v", first)
	}
	if first.Signal != 82 { // -59 → 2*(100-59)=82
		t.Errorf("signal = %d", first.Signal)
	}
	if first.Security != "WPA2(PSK/AES, 5/6)" {
		t.Errorf("security = %q", first.Security)
	}
	if nets[2].Signal != 100 {
		t.Errorf("clamped signal = %d", nets[2].Signal)
	}
}

func TestParseDarwinProxyAndBypass(t *testing.T) {
	out := `Enabled: Yes
Server: proxy.example.com
Port: 8080
Authenticated Proxy Enabled: 0
`
	on, host, port := parseDarwinProxy(out)
	if !on || host != "proxy.example.com" || port != 8080 {
		t.Errorf("proxy = %v %s %d", on, host, port)
	}
	if off, _, _ := parseDarwinProxy("Enabled: No\nServer: x\nPort: 1\n"); off {
		t.Error("enabled should be false")
	}
	bypass := parseDarwinBypass("*.local\n169.254/16\nThere aren't any bypass domains set.")
	if !reflect.DeepEqual(bypass, []string{"*.local", "169.254/16"}) {
		t.Errorf("bypass = %v", bypass)
	}
}

func TestMaskPrefixConversions(t *testing.T) {
	ones, ok := maskToPrefixLen("255.255.255.0")
	if !ok || ones != 24 {
		t.Fatalf("ones = %d ok=%v", ones, ok)
	}
	if _, ok := maskToPrefixLen("255.0.255.0"); ok {
		t.Error("non-contiguous mask should fail")
	}
	mask, ok := prefixLenToMask(20)
	if !ok || mask != "255.255.240.0" {
		t.Errorf("mask = %q ok=%v", mask, ok)
	}
}

func TestParseSystemProfilerAirport(t *testing.T) {
	out := `Wi-Fi:

      Software Versions:
          CoreWLAN: 16.0 (1657)
      Interfaces:
        en0:
          Card Type: Wi-Fi  (0x14E4, 0x7BF)
          Status: Connected
          Current Network Information:
            HomeNet:
              PHY Mode: 802.11ac
              Channel: 48 (5GHz, 80MHz)
              Security: WPA2 Personal
              Signal / Noise: -55 dBm / -92 dBm
          Other Local Wi-Fi Networks:
            Neighbor 2.4G:
              PHY Mode: 802.11b/g/n
              Channel: 1 (2GHz, 40MHz)
              Security: None
              Signal / Noise: -43 dBm / -93 dBm
            <redacted>:
              Security: WPA3 Personal
        awdl0:
          Card Type: AWDL
`
	current, nets, redacted := parseSystemProfilerAirport(out)
	if current != "HomeNet" {
		t.Errorf("current = %q", current)
	}
	if redacted != 1 {
		t.Errorf("redacted = %d", redacted)
	}
	if len(nets) != 2 {
		t.Fatalf("len = %d: %+v", len(nets), nets)
	}
	if !nets[0].Current || nets[0].SSID != "HomeNet" || nets[0].Channel != "48" || nets[0].Signal != 90 {
		t.Errorf("current net = %+v", nets[0])
	}
	if nets[1].Current || nets[1].SSID != "Neighbor 2.4G" || nets[1].Security != "" || nets[1].Signal != 100 {
		t.Errorf("neighbor = %+v", nets[1])
	}
	for _, n := range nets {
		if n.SSID == "awdl0" || n.SSID == "en0" {
			t.Errorf("interface name leaked as SSID: %+v", n)
		}
	}
}

func TestParseWdutilInfo(t *testing.T) {
	out := `WIFI INFORMATION:

     NIC: en0
     CURRENT NETWORK:
     SSID: HomeNet
     BSSID: aa:bb:cc:dd:ee:ff
     CHANNEL: 149
     RSSI: -55
     NOISE: -90
`
	net := parseWdutilInfo(out)
	if net == nil {
		t.Fatal("net = nil")
	}
	if net.SSID != "HomeNet" || !net.Current || net.Channel != "149" || net.Signal != 90 || net.BSSID != "aa:bb:cc:dd:ee:ff" {
		t.Errorf("net = %+v", net)
	}
	if redacted := parseWdutilInfo("     SSID: <redacted>\n"); redacted != nil {
		t.Errorf("redacted should return nil: %+v", redacted)
	}
	if none := parseWdutilInfo("     NOISE: -90\n"); none != nil {
		t.Errorf("no SSID should return nil: %+v", none)
	}
}
