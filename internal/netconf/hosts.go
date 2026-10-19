package netconf

import (
	"net"
	"os"
	"path/filepath"
	"strings"
)

// The hosts manager owns only the marked section of the hosts file; every
// other line is preserved byte-for-byte. Writes go through a temp file and
// rename so a crash never leaves a half-written file behind.

const (
	hostsBeginMarker = "# BEGIN netcfg"
	hostsEndMarker   = "# END netcfg"
)

type fileHosts struct {
	path string
}

// NewHosts returns the hosts manager for the platform's hosts file.
func NewHosts() HostsManager { return &fileHosts{path: HostsPath()} }

// NewHostsAt returns a hosts manager bound to an explicit path (tests).
func NewHostsAt(path string) HostsManager { return &fileHosts{path: path} }

// Entries lists every mapping line of the hosts file, flagging the ones
// inside the managed section.
func (h *fileHosts) Entries() ([]HostEntry, error) {
	content, err := os.ReadFile(h.path)
	if err != nil {
		return nil, newError(CodeCommandFailed, "read hosts file: %v", err)
	}
	var entries []HostEntry
	managed := false
	for _, line := range strings.Split(string(content), "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == hostsBeginMarker {
			managed = true
			continue
		}
		if trimmed == hostsEndMarker {
			managed = false
			continue
		}
		fields := strings.Fields(trimmed)
		if len(fields) < 2 || strings.HasPrefix(trimmed, "#") {
			continue
		}
		if net.ParseIP(fields[0]) == nil {
			continue
		}
		entries = append(entries, HostEntry{IP: fields[0], Hostnames: fields[1:], Managed: managed})
	}
	return entries, nil
}

// Add maps hostname to ip inside the managed section, creating the section
// if needed. An existing managed mapping for the hostname is updated.
func (h *fileHosts) Add(ip, hostname string) error {
	if net.ParseIP(ip) == nil {
		return newError(CodeInvalidArgument, "%q is not an IP address", ip)
	}
	if !validHostname(hostname) {
		return newError(CodeInvalidArgument, "%q is not a valid hostname", hostname)
	}
	lines, err := h.readlines()
	if err != nil {
		return err
	}
	begin, end := managedRange(lines)
	if begin >= 0 {
		for i := begin + 1; i < end; i++ {
			fields := strings.Fields(strings.TrimSpace(lines[i]))
			if len(fields) >= 2 && containsString(fields[1:], hostname) {
				names := removeString(fields[1:], hostname)
				names = append(names, hostname)
				lines[i] = ip + " " + strings.Join(names, " ")
				return h.writelines(lines)
			}
		}
		// append before the END marker
		insert := ip + " " + hostname
		lines = append(lines[:end], append([]string{insert}, lines[end:]...)...)
		return h.writelines(lines)
	}
	if len(lines) > 0 && strings.TrimSpace(lines[len(lines)-1]) != "" {
		lines = append(lines, "")
	}
	lines = append(lines, hostsBeginMarker, ip+" "+hostname, hostsEndMarker)
	return h.writelines(lines)
}

// Remove drops hostname from the managed section only; hostnames managed
// elsewhere are untouched.
func (h *fileHosts) Remove(hostname string) error {
	lines, err := h.readlines()
	if err != nil {
		return err
	}
	begin, end := managedRange(lines)
	if begin < 0 {
		return newError(CodeNotFound, "%q is not managed by netcfg", hostname)
	}
	found := false
	kept := lines[:begin+1]
	for i := begin + 1; i < end; i++ {
		fields := strings.Fields(strings.TrimSpace(lines[i]))
		if len(fields) < 2 || !containsString(fields[1:], hostname) {
			kept = append(kept, lines[i])
			continue
		}
		rest := removeString(fields[1:], hostname)
		found = true
		if len(rest) > 0 {
			kept = append(kept, fields[0]+" "+strings.Join(rest, " "))
		}
	}
	if !found {
		return newError(CodeNotFound, "%q is not managed by netcfg", hostname)
	}
	kept = append(kept, lines[end:]...)
	return h.writelines(kept)
}

// managedRange returns the line indexes of the marker lines, or (-1, -1).
func managedRange(lines []string) (begin, end int) {
	begin, end = -1, -1
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == hostsBeginMarker && begin < 0 {
			begin = i
		} else if trimmed == hostsEndMarker && begin >= 0 {
			end = i
			return begin, end
		}
	}
	return -1, -1
}

func (h *fileHosts) readlines() ([]string, error) {
	content, err := os.ReadFile(h.path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, newError(CodeCommandFailed, "read hosts file: %v", err)
	}
	lines := strings.Split(strings.TrimSuffix(string(content), "\n"), "\n")
	if len(lines) == 1 && lines[0] == "" {
		return nil, nil
	}
	return lines, nil
}

func (h *fileHosts) writelines(lines []string) error {
	content := strings.Join(lines, "\n") + "\n"
	dir := filepath.Dir(h.path)
	mode := os.FileMode(0o644)
	if info, err := os.Stat(h.path); err == nil {
		mode = info.Mode().Perm()
	}
	tmp, err := os.CreateTemp(dir, ".netcfg-hosts-*")
	if err != nil {
		return newError(CodeCommandFailed, "create temp file: %v", err)
	}
	path := tmp.Name()
	defer os.Remove(path)
	if _, err := tmp.WriteString(content); err != nil {
		tmp.Close()
		return newError(CodeCommandFailed, "write temp file: %v", err)
	}
	if err := tmp.Chmod(mode); err != nil {
		tmp.Close()
		return newError(CodeCommandFailed, "chmod temp file: %v", err)
	}
	if err := tmp.Close(); err != nil {
		return newError(CodeCommandFailed, "close temp file: %v", err)
	}
	if err := os.Rename(path, h.path); err != nil {
		return newError(CodeCommandFailed, "replace hosts file: %v", err)
	}
	return nil
}

func containsString(list []string, s string) bool {
	for _, item := range list {
		if item == s {
			return true
		}
	}
	return false
}

func removeString(list []string, s string) []string {
	var out []string
	for _, item := range list {
		if item != s {
			out = append(out, item)
		}
	}
	return out
}

// validHostname accepts the conservative subset: letters, digits, dots and
// hyphens, no empty labels, 253 bytes max.
func validHostname(s string) bool {
	if s == "" || len(s) > 253 {
		return false
	}
	for _, label := range strings.Split(s, ".") {
		if label == "" || len(label) > 63 {
			return false
		}
		for _, r := range label {
			switch {
			case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '-':
			default:
				return false
			}
		}
	}
	return true
}
