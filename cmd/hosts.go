package cmd

import (
	"fmt"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/LinuxSuRen/netcfg/internal/netconf"
)

var hostsListCmd = &cobra.Command{
	Use:   "list",
	Short: "List hosts file entries (managed ones are flagged)",
	RunE: func(cmd *cobra.Command, args []string) error {
		entries, err := netconf.NewHosts().Entries()
		if err != nil {
			return exitErr(cmd, err)
		}
		if outputJSON {
			printJSON(cmd.OutOrStdout(), entries)
			return nil
		}
		w := tabwriter.NewWriter(cmd.OutOrStdout(), 2, 4, 2, ' ', 0)
		fmt.Fprintln(w, "IP\tHOSTNAMES\tSOURCE")
		for _, e := range entries {
			source := "system"
			if e.Managed {
				source = "netcfg"
			}
			fmt.Fprintf(w, "%s\t%s\t%s\n", e.IP, strings.Join(e.Hostnames, ", "), source)
		}
		return w.Flush()
	},
}

var hostsAddCmd = &cobra.Command{
	Use:   "add <ip> <hostname>",
	Short: "Map a hostname to an IP inside the netcfg-managed hosts section",
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := netconf.NewHosts().Add(args[0], args[1]); err != nil {
			return exitErr(cmd, err)
		}
		fmt.Fprintf(cmd.OutOrStdout(), "%s -> %s\n", args[1], args[0])
		return nil
	},
}

var hostsRemoveCmd = &cobra.Command{
	Use:   "remove <hostname>",
	Short: "Drop a hostname from the netcfg-managed hosts section",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := netconf.NewHosts().Remove(args[0]); err != nil {
			return exitErr(cmd, err)
		}
		fmt.Fprintf(cmd.OutOrStdout(), "removed %s\n", args[0])
		return nil
	},
}

var hostsCmd = &cobra.Command{
	Use:   "hosts",
	Short: "Inspect and manage hosts file entries",
	Long: `Inspect and manage hosts file entries.

netcfg only edits the marked section of the hosts file; every other line,
including comments and foreign entries, is preserved as-is.`,
}

func init() {
	hostsCmd.AddCommand(hostsListCmd, hostsAddCmd, hostsRemoveCmd)
	rootCmd.AddCommand(hostsCmd)
}
