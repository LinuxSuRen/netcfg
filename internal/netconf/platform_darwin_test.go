//go:build darwin

package netconf

import (
	"path/filepath"
	"strings"
	"testing"
)

// cmdRunner returns canned output per command name (basename of argv[0]).
type cmdRunner map[string]string

func (c cmdRunner) Run(name string, _ ...string) (string, error) {
	return c[filepath.Base(name)], nil
}

const airportWarning = "WARNING: The airport command line tool is deprecated and will be removed in a future release."

const sysprofFixture = `Wi-Fi:

      Interfaces:
        en0:
          Status: Connected
          Current Network Information:
            HomeNet:
              PHY Mode: 802.11ac
              Channel: 48 (5GHz, 80MHz)
              Security: WPA2 Personal
              Signal / Noise: -55 dBm / -92 dBm
          Other Local Wi-Fi Networks:
            Neighbor:
              PHY Mode: 802.11b/g/n
              Channel: 1 (2GHz, 40MHz)
              Security: None
              Signal / Noise: -43 dBm / -93 dBm
            <redacted>:
              Security: WPA3 Personal
        awdl0:
          Card Type: AWDL
`

// TestDarwinScanWifiAirportRows keeps the airport fast path honest.
func TestDarwinScanWifiAirportRows(t *testing.T) {
	m := &darwinManager{query: cmdRunner{
		"airport": "     SSID BSSID             RSSI CHANNEL HT CC SECURITY\n" +
			"     Home aa:bb:cc:dd:ee:ff -55  6       Y  US WPA2(PSK/AES)",
	}}
	nets, err := m.ScanWifi()
	if err != nil {
		t.Fatal(err)
	}
	if len(nets) != 1 || nets[0].SSID != "Home" || nets[0].Signal != 90 {
		t.Errorf("nets = %+v", nets)
	}
}

// TestDarwinScanWifiSystemProfilerFallback covers macOS 15+: airport empty,
// system_profiler lists networks (a redacted neighbor must be dropped).
func TestDarwinScanWifiSystemProfilerFallback(t *testing.T) {
	m := &darwinManager{query: cmdRunner{
		"airport":         airportWarning,
		"system_profiler": sysprofFixture,
	}}
	nets, err := m.ScanWifi()
	if err != nil {
		t.Fatal(err)
	}
	if len(nets) != 2 {
		t.Fatalf("len = %d: %+v", len(nets), nets)
	}
	if !nets[0].Current || nets[0].SSID != "HomeNet" || nets[0].Channel != "48" {
		t.Errorf("current = %+v", nets[0])
	}
	if nets[1].SSID != "Neighbor" || nets[1].Security != "" || nets[1].Signal != 100 {
		t.Errorf("neighbor = %+v", nets[1])
	}
}

// TestDarwinScanWifiAllRedacted covers the fully redacted case: netcfg must
// trigger the Location authorization helper itself (here a recording stub)
// and return an actionable prompt message.
func TestDarwinScanWifiAllRedacted(t *testing.T) {
	triggered := false
	m := &darwinManager{
		query: cmdRunner{
			"airport":         airportWarning,
			"system_profiler": "          Other Local Wi-Fi Networks:\n            <redacted>:\n              Security: WPA2 Personal\n",
			"wdutil":          "     SSID: <redacted>\n",
		},
		requestLocation: func() bool { triggered = true; return true },
	}
	_, err := m.ScanWifi()
	if err == nil {
		t.Fatal("expected an error")
	}
	if CodeOf(err) != CodePermissionDenied {
		t.Errorf("code = %s (%v)", CodeOf(err), err)
	}
	if !triggered {
		t.Error("location authorization helper was not triggered")
	}
	if !strings.Contains(err.Error(), "Allow") {
		t.Errorf("message lacks prompt guidance: %v", err)
	}
}
