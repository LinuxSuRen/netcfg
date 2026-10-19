package netconf

import (
	"net"
	"strings"
)

// Input validation shared by every platform backend. Values that end up as
// arguments of system commands (interface names, SSIDs, proxy hosts) are
// always passed as separate argv elements and never through a shell; these
// checks additionally rule out argument confusion (values a receiving tool
// could misread as flags) and control characters.

// validSSID accepts printable strings that no receiving tool can mistake
// for an option flag; 32 bytes is the 802.11 SSID limit.
func validSSID(s string) bool {
	if s == "" || len(s) > 32 {
		return false
	}
	if s == "-" || strings.HasPrefix(s, "-") {
		return false
	}
	for _, r := range s {
		if r < 0x20 || r == 0x7f {
			return false
		}
	}
	return true
}

// validProxyHost accepts an IP address or a plain hostname.
func validProxyHost(s string) bool {
	if s == "" {
		return false
	}
	if net.ParseIP(s) != nil {
		return true
	}
	return validHostname(s)
}
