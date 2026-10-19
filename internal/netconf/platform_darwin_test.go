//go:build darwin

package netconf

import (
	"strings"
	"testing"
)

// stubRunner returns canned output for any command.
type stubRunner struct{ out string }

func (s stubRunner) Run(string, ...string) (string, error) { return s.out, nil }

// TestDarwinScanWifiNeedsPrivileges pins the macOS behaviour that an
// unprivileged airport scan (warning only, no rows) surfaces as
// permission_denied instead of an empty list.
func TestDarwinScanWifiNeedsPrivileges(t *testing.T) {
	if airportPath() == "" {
		t.Skip("airport tool not present")
	}
	m := &darwinManager{query: stubRunner{out: "WARNING: The airport command line tool is deprecated and will be removed in a future release."}}
	_, err := m.ScanWifi()
	if err == nil {
		t.Fatal("expected an error for an unprivileged scan")
	}
	if CodeOf(err) != CodePermissionDenied {
		t.Errorf("code = %s (%v), want %s", CodeOf(err), err, CodePermissionDenied)
	}
}

func TestDarwinScanWifiParsesRows(t *testing.T) {
	if airportPath() == "" {
		t.Skip("airport tool not present")
	}
	out := "     SSID BSSID             RSSI CHANNEL HT CC SECURITY\n" +
		"     Home aa:bb:cc:dd:ee:ff -55  6       Y  US WPA2(PSK/AES)"
	m := &darwinManager{query: stubRunner{out: out}}
	nets, err := m.ScanWifi()
	if err != nil {
		t.Fatal(err)
	}
	if len(nets) != 1 || nets[0].SSID != "Home" || nets[0].Signal != 90 {
		t.Errorf("nets = %+v", nets)
	}
	if !strings.Contains(nets[0].Security, "WPA2") {
		t.Errorf("security = %q", nets[0].Security)
	}
}
