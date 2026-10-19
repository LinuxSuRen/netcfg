// netcfg views and changes the local machine's network configuration on
// macOS, Windows and Linux, from the terminal or a small embedded web UI.
package main

import (
	"github.com/LinuxSuRen/netcfg/cmd"
)

// set at release time via -ldflags
var (
	version = "dev"
	commit  = "none"
)

func main() {
	cmd.SetVersion(version, commit)
	cmd.Execute()
}
