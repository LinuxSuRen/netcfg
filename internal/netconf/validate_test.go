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
