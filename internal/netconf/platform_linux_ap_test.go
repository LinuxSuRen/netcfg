//go:build linux

package netconf

import (
	"strings"
	"testing"
)

// fakeRunner answers fixed outputs by full command string; anything unknown
// fails the test loudly by returning an error.
type fakeRunner struct {
	outputs map[string]string
}

func (f *fakeRunner) Run(name string, args ...string) (string, error) {
	full := name + " " + strings.Join(args, " ")
	if out, ok := f.outputs[full]; ok {
		return out, nil
	}
	return "", newError(CodeCommandFailed, "unexpected command in test: %s", full)
}

const deviceShowOut = `GENERAL.DEVICE:wlan0
GENERAL.TYPE:wifi
GENERAL.STATE:100 (connected)
GENERAL.CONNECTION:netcfg-ap
`

func newAPManager(connectionList string) (*linuxManager, *DryRunner) {
	query := &fakeRunner{outputs: map[string]string{
		"nmcli -t -e no -f GENERAL.DEVICE,GENERAL.TYPE,GENERAL.HWADDR,GENERAL.MTU,GENERAL.STATE,GENERAL.CONNECTION,IP4.ADDRESS,IP4.GATEWAY,IP4.DNS device show": deviceShowOut,
		"nmcli -t -f NAME,DEVICE,ipv4.method connection show": "",
		"nmcli -t -e no -f ACTIVE,SSID device wifi":           "",
		"nmcli -t -f NAME connection show":                    connectionList,
	}}
	dry := &DryRunner{}
	return &linuxManager{query: query, mutate: dry, hasNM: true}, dry
}

func TestStartWifiAPCommands(t *testing.T) {
	mgr, dry := newAPManager("Home WiFi\nWired connection 1")
	if err := mgr.StartWifiAP("wlan0", "test-ap", "secret123",
		WifiAPOptions{Channel: 6, Gateway: "192.168.42.1"}); err != nil {
		t.Fatalf("start: %v", err)
	}
	want := []string{
		"nmcli connection add type wifi ifname wlan0 con-name netcfg-ap ssid test-ap " +
			"802-11-wireless.mode ap 802-11-wireless.band bg 802-11-wireless.channel 6 " +
			"802-11-wireless-security.key-mgmt wpa-psk wifi-sec.psk secret123 " +
			"ipv4.method shared ipv4.addresses 192.168.42.1/24",
		"nmcli connection up netcfg-ap",
	}
	if len(dry.Commands) != len(want) {
		t.Fatalf("commands = %v", dry.Commands)
	}
	for i := range want {
		if dry.Commands[i] != want[i] {
			t.Errorf("cmd[%d]:\n got %s\nwant %s", i, dry.Commands[i], want[i])
		}
	}
}

func TestStartWifiAPReplacesLeftover(t *testing.T) {
	mgr, dry := newAPManager("netcfg-ap")
	if err := mgr.StartWifiAP("", "open-ap", "", WifiAPOptions{}); err != nil {
		t.Fatalf("start: %v", err)
	}
	if dry.Commands[0] != "nmcli connection delete netcfg-ap" {
		t.Errorf("leftover cleanup missing: %v", dry.Commands)
	}
	add := dry.Commands[1]
	if strings.Contains(add, "key-mgmt") || strings.Contains(add, "psk") {
		t.Errorf("open network must not carry security settings: %s", add)
	}
	if !strings.Contains(add, "ifname wlan0") {
		t.Errorf("default wifi device not picked: %s", add)
	}
}

func TestStartWifiAP5GHz(t *testing.T) {
	mgr, dry := newAPManager("")
	if err := mgr.StartWifiAP("wlan0", "ap5", "secret123",
		WifiAPOptions{Band: "a", Channel: 36}); err != nil {
		t.Fatalf("start: %v", err)
	}
	add := dry.Commands[0]
	if !strings.Contains(add, "802-11-wireless.band a 802-11-wireless.channel 36") {
		t.Errorf("5 GHz flags missing: %s", add)
	}
}

func TestStopWifiAPCommands(t *testing.T) {
	mgr, dry := newAPManager("netcfg-ap")
	if err := mgr.StopWifiAP(WifiAPOptions{}); err != nil {
		t.Fatalf("stop: %v", err)
	}
	want := []string{"nmcli connection down netcfg-ap", "nmcli connection delete netcfg-ap"}
	for i := range want {
		if dry.Commands[i] != want[i] {
			t.Errorf("cmd[%d] = %s want %s", i, dry.Commands[i], want[i])
		}
	}
}

func TestStopWifiAPMissing(t *testing.T) {
	mgr, _ := newAPManager("")
	err := mgr.StopWifiAP(WifiAPOptions{})
	if err == nil {
		t.Fatal("expected not-found error")
	}
	if CodeOf(err) != CodeNotFound {
		t.Errorf("code = %s want %s", CodeOf(err), CodeNotFound)
	}
}

func TestStartWifiAPWithoutNM(t *testing.T) {
	mgr := &linuxManager{query: &fakeRunner{}, mutate: &DryRunner{}, hasNM: false}
	err := mgr.StartWifiAP("wlan0", "ap", "secret123", WifiAPOptions{})
	if CodeOf(err) != CodeUnsupported {
		t.Errorf("code = %s want %s", CodeOf(err), CodeUnsupported)
	}
}
