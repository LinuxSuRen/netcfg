package cmd

import (
	"fmt"
	"strings"
	"syscall"
	"text/tabwriter"

	"github.com/spf13/cobra"
	"golang.org/x/term"

	"github.com/LinuxSuRen/netcfg/internal/netconf"
)

var wifiListCmd = &cobra.Command{
	Use:   "list",
	Short: "Scan for visible Wi-Fi networks",
	RunE: func(cmd *cobra.Command, args []string) error {
		nets, err := netconf.NewManager().ScanWifi()
		if err != nil {
			return exitErr(cmd, err)
		}
		if outputJSON {
			printJSON(cmd.OutOrStdout(), nets)
			return nil
		}
		w := tabwriter.NewWriter(cmd.OutOrStdout(), 2, 4, 2, ' ', 0)
		fmt.Fprintln(w, "SSID\tSIGNAL\tSECURITY\tCHANNEL\tCURRENT")
		for _, net := range nets {
			current := ""
			if net.Current {
				current = "*"
			}
			fmt.Fprintf(w, "%s\t%d%%\t%s\t%s\t%s\n", net.SSID, net.Signal, net.Security, net.Channel, current)
		}
		if err := w.Flush(); err == nil && len(nets) == 0 {
			fmt.Fprintln(cmd.ErrOrStderr(), "no networks found; scanning may need administrator privileges on this system")
		}
		return nil
	},
}

var (
	wifiConnectIface    string
	wifiConnectPassword string
	wifiConnectDry      bool
)

var wifiConnectCmd = &cobra.Command{
	Use:   "connect <ssid>",
	Short: "Join a Wi-Fi network",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		ssid := args[0]
		password := wifiConnectPassword
		if password == "" {
			fmt.Fprint(cmd.ErrOrStderr(), "password (input hidden, empty for open network): ")
			raw, err := term.ReadPassword(int(syscall.Stdin))
			fmt.Fprintln(cmd.ErrOrStderr())
			if err != nil {
				return exitErr(cmd, fmt.Errorf("read password: %v", err))
			}
			password = strings.TrimSpace(string(raw))
		}
		mgr := newManager(wifiConnectDry)
		if err := mgr.ConnectWifi(wifiConnectIface, ssid, password); err != nil {
			return exitErr(cmd, err)
		}
		if wifiConnectDry {
			reportDry(mgr)
			return nil
		}
		fmt.Fprintf(cmd.OutOrStdout(), "requested to join %s\n", ssid)
		return nil
	},
}

var wifiCmd = &cobra.Command{
	Use:   "wifi",
	Short: "Scan and join Wi-Fi networks",
}

func init() {
	wifiCmd.AddCommand(wifiListCmd)
	f := wifiConnectCmd.Flags()
	f.StringVar(&wifiConnectIface, "iface", "", "interface to use (defaults to the platform's Wi-Fi interface)")
	f.StringVar(&wifiConnectPassword, "password", "", "pre-shared key (prompted when omitted)")
	f.BoolVar(&wifiConnectDry, "dry-run", false, "print the system commands instead of running them")
	wifiCmd.AddCommand(wifiConnectCmd)
	rootCmd.AddCommand(wifiCmd)
}
