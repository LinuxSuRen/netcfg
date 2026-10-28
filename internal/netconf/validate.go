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

// validateWifiAPArgs checks hotspot parameters shared by every platform:
// SSID sanity as for joining, the 8..63 bytes WPA2 requires for the PSK,
// band/channel pairing and an optional gateway IPv4.
func validateWifiAPArgs(ssid, password string, opts WifiAPOptions) error {
	if !validSSID(ssid) {
		return newError(CodeInvalidArgument, "invalid SSID %q", ssid)
	}
	if password != "" && (len(password) < 8 || len(password) > 63) {
		return newError(CodeInvalidArgument,
			"WPA2 passphrase must be 8..63 bytes (got %d); use an empty password for an open network", len(password))
	}
	switch opts.Band {
	case "", "bg", "a":
	default:
		return newError(CodeInvalidArgument, "band must be \"bg\" or \"a\" (got %q)", opts.Band)
	}
	if opts.Channel < 0 || opts.Channel > 196 {
		return newError(CodeInvalidArgument, "channel %d out of range", opts.Channel)
	}
	if opts.Channel != 0 && opts.Band == "bg" && opts.Channel > 14 {
		return newError(CodeInvalidArgument, "2.4 GHz band supports channels 1..14 (got %d)", opts.Channel)
	}
	if opts.Channel != 0 && opts.Band == "a" && opts.Channel < 32 {
		return newError(CodeInvalidArgument, "5 GHz band starts at channel 32 (got %d)", opts.Channel)
	}
	if opts.Gateway != "" && net.ParseIP(opts.Gateway) == nil {
		return newError(CodeInvalidArgument, "%q is not an IP address", opts.Gateway)
	}
	return nil
}
