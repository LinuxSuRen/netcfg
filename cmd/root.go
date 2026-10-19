// Package cmd wires the cobra CLI of netcfg.
package cmd

import (
	"encoding/json"
	"fmt"
	"io"
	"os"

	"github.com/spf13/cobra"

	"github.com/LinuxSuRen/netcfg/internal/netconf"
)

// Version metadata, set via -ldflags.
var (
	version = "dev"
	commit  = "none"
)

// outputJSON is the shared -o/--output switch value.
var outputJSON bool

var rootCmd = &cobra.Command{
	Use:   "netcfg",
	Short: "View and change local network configuration (macOS / Windows / Linux)",
	Long: `netcfg views and changes this machine's network configuration:
interfaces, IP/DHCP, DNS, Wi-Fi, the hosts file and the desktop proxy.

It works on macOS (networksetup), Windows (netsh / PowerShell) and Linux
(NetworkManager; read-only fallback via iproute2). Mutating commands need
administrator privileges and support --dry-run to preview the exact system
commands. The same functionality is available over HTTP with ` + "`netcfg serve`" + `.`,
	SilenceUsage:  true,
	SilenceErrors: true,
}

// Execute runs the CLI; exit code convention: 0 ok, 1 failure, 2 usage.
func Execute() {
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

func init() {
	rootCmd.PersistentFlags().BoolVarP(&outputJSON, "output", "o", false, "print JSON instead of a table")
	applyVersion()
}

// SetVersion records build metadata (called from main with -ldflags values).
func SetVersion(v, c string) {
	version, commit = v, c
	applyVersion()
}

func applyVersion() {
	rootCmd.Version = version
	rootCmd.SetVersionTemplate("netcfg {{.Version}} (commit " + commit + ")\n")
}

// newManager builds the platform manager, dry-run when requested.
func newManager(dry bool) netconf.Manager {
	if dry {
		return netconf.NewDryManager()
	}
	return netconf.NewManager()
}

// dryRunner is implemented by dry managers to report planned commands.
type dryRunner interface {
	DryCommands() []string
}

// reportDry prints the commands a dry run recorded.
func reportDry(mgr netconf.Manager) {
	if d, ok := mgr.(dryRunner); ok {
		fmt.Println("dry run; would run:")
		for _, c := range d.DryCommands() {
			fmt.Println("  " + c)
		}
	}
}

// printJSON marshals v for CLI output.
func printJSON(w io.Writer, v any) {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	_ = enc.Encode(v)
}

// exitErr prints an error and returns the failure code for RunE.
func exitErr(cmd *cobra.Command, err error) error {
	fmt.Fprintln(cmd.ErrOrStderr(), "error:", err)
	return err
}
