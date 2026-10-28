package netconf

import "testing"

func TestValidSSID(t *testing.T) {
	cases := []struct {
		in   string
		want bool
	}{
		{"HomeNet", true},
		{"办公网 5G", true},
		{"with space", true},
		{"", false},
		{"-flag", false},
		{"--", false},
		{"bad\x00nul", false},
		{"new\nline", false},
		{string(make([]byte, 33)), false}, // 33 NUL bytes: too long, control chars
	}
	for _, c := range cases {
		if got := validSSID(c.in); got != c.want {
			t.Errorf("validSSID(%q) = %v, want %v", c.in, got, c.want)
		}
	}
}

func TestValidProxyHost(t *testing.T) {
	if !validProxyHost("127.0.0.1") {
		t.Error("ip should be valid")
	}
	if !validProxyHost("proxy.example.com") {
		t.Error("hostname should be valid")
	}
	for _, bad := range []string{"", "has space", "a;b", "host;rm"} {
		if validProxyHost(bad) {
			t.Errorf("%q should be invalid", bad)
		}
	}
}

func TestValidateWifiAPArgs(t *testing.T) {
	valid := []struct {
		name     string
		ssid     string
		password string
		opts     WifiAPOptions
	}{
		{"wpa2 defaults", "test-ap", "secret123", WifiAPOptions{}},
		{"open network", "test-ap", "", WifiAPOptions{}},
		{"5 GHz channel", "test-ap", "secret123", WifiAPOptions{Band: "a", Channel: 36}},
		{"2.4 GHz channel", "test-ap", "secret123", WifiAPOptions{Band: "bg", Channel: 11}},
		{"gateway", "test-ap", "secret123", WifiAPOptions{Gateway: "192.168.42.1"}},
	}
	for _, c := range valid {
		if err := validateWifiAPArgs(c.ssid, c.password, c.opts); err != nil {
			t.Errorf("%s: unexpected error: %v", c.name, err)
		}
	}
	invalid := []struct {
		name     string
		ssid     string
		password string
		opts     WifiAPOptions
	}{
		{"empty ssid", "", "secret123", WifiAPOptions{}},
		{"flag ssid", "-x", "secret123", WifiAPOptions{}},
		{"short psk", "test-ap", "short", WifiAPOptions{}},
		{"long psk", "test-ap", string(make([]byte, 64)), WifiAPOptions{}},
		{"unknown band", "test-ap", "secret123", WifiAPOptions{Band: "ac"}},
		{"2.4 channel out of range", "test-ap", "secret123", WifiAPOptions{Band: "bg", Channel: 36}},
		{"5 GHz channel under range", "test-ap", "secret123", WifiAPOptions{Band: "a", Channel: 6}},
		{"bad gateway", "test-ap", "secret123", WifiAPOptions{Gateway: "not-an-ip"}},
	}
	for _, c := range invalid {
		if err := validateWifiAPArgs(c.ssid, c.password, c.opts); err == nil {
			t.Errorf("%s: expected error, got nil", c.name)
		} else if CodeOf(err) != CodeInvalidArgument {
			t.Errorf("%s: code = %s want %s", c.name, CodeOf(err), CodeInvalidArgument)
		}
	}
}
