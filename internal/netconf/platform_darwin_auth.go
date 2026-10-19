//go:build darwin

package netconf

import (
	_ "embed"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
)

// location_auth.m.txt asks for Location authorization once on behalf of the
// terminal app; macOS records Wi-Fi SSID visibility per responsible process,
// so the grant lands on the terminal even though netcfg triggers it.
//
//go:embed location_auth.m.txt
var locationAuthSource string

var (
	locationAuthOnce      sync.Once
	locationAuthTriggered bool
)

// requestLocationAuthOnce compiles the embedded helper with the local clang
// and launches it detached. The system prompt appears immediately; the
// helper exits by itself once answered (or after 90s). It runs at most once
// per process so repeated scans do not spam prompts. Returns false when the
// helper could not be launched (e.g. no Xcode CLT installed) so the caller
// falls back to textual guidance.
func requestLocationAuthOnce() bool {
	locationAuthOnce.Do(func() {
		locationAuthTriggered = launchLocationAuthHelper()
	})
	return locationAuthTriggered
}

func launchLocationAuthHelper() bool {
	clang, err := exec.LookPath("clang")
	if err != nil {
		return false
	}
	dir, err := os.MkdirTemp("", "netcfg-location-")
	if err != nil {
		return false
	}
	src := filepath.Join(dir, "location-auth.m")
	bin := filepath.Join(dir, "location-auth")
	if err := os.WriteFile(src, []byte(locationAuthSource), 0o644); err != nil {
		return false
	}
	compile := exec.Command(clang, "-framework", "Foundation", "-framework",
		"CoreLocation", src, "-o", bin)
	if out, err := compile.CombinedOutput(); err != nil {
		_ = out
		return false
	}
	// Detached: the HTTP request must not block on the user answering the
	// prompt. The helper self-terminates.
	helper := exec.Command(bin)
	helper.Stdout = nil
	helper.Stderr = nil
	if err := helper.Start(); err != nil {
		return false
	}
	go func() { _ = helper.Wait() }() // reap, avoid a zombie
	return true
}
