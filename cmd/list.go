package cmd

import (
	"fmt"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/LinuxSuRen/netcfg/internal/netconf"
)

var listCmd = &cobra.Command{
	Use:     "list",
	Aliases: []string{"ls", "interfaces"},
	Short:   "List network interfaces with their IP, gateway and DNS",
	RunE: func(cmd *cobra.Command, args []string) error {
		mgr := netconf.NewManager()
		ifaces, err := mgr.Interfaces()
		if err != nil {
			return exitErr(cmd, err)
		}
		if outputJSON {
			printJSON(cmd.OutOrStdout(), ifaces)
			return nil
		}
		w := tabwriter.NewWriter(cmd.OutOrStdout(), 2, 4, 2, ' ', 0)
		fmt.Fprintln(w, "NAME\tTYPE\tSTATE\tMODE\tADDRESSES\tGATEWAY\tDNS")
		for _, iface := range ifaces {
			var addrs []string
			for _, ip := range iface.IPs {
				if ip.Family == "ipv4" {
					addrs = append(addrs, ip.Address)
				}
			}
			mode := "static"
			if iface.DHCP {
				mode = "dhcp"
			}
			state := "down"
			if iface.Up {
				state = "up"
			}
			fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\t%s\n",
				iface.Name, iface.Type, state, mode,
				strings.Join(addrs, ", "), iface.Gateway, strings.Join(iface.DNS, ","))
		}
		return w.Flush()
	},
}

var showCmd = &cobra.Command{
	Use:   "show <interface>",
	Short: "Show one interface in detail",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		mgr := netconf.NewManager()
		ifaces, err := mgr.Interfaces()
		if err != nil {
			return exitErr(cmd, err)
		}
		for _, iface := range ifaces {
			if strings.EqualFold(iface.Name, args[0]) {
				if outputJSON {
					printJSON(cmd.OutOrStdout(), iface)
					return nil
				}
				printInterface(cmd, iface)
				return nil
			}
		}
		return exitErr(cmd, fmt.Errorf("interface %q not found", args[0]))
	},
}

func printInterface(cmd *cobra.Command, iface netconf.Interface) {
	out := cmd.OutOrStdout()
	fmt.Fprintf(out, "name:    %s\n", iface.Name)
	fmt.Fprintf(out, "type:    %s\n", iface.Type)
	fmt.Fprintf(out, "state:   %s\n", onOff(iface.Up))
	if iface.HardwareAddr != "" {
		fmt.Fprintf(out, "mac:     %s\n", iface.HardwareAddr)
	}
	if iface.MTU > 0 {
		fmt.Fprintf(out, "mtu:     %d\n", iface.MTU)
	}
	mode := "static"
	if iface.DHCP {
		mode = "dhcp"
	}
	fmt.Fprintf(out, "mode:    %s\n", mode)
	for _, ip := range iface.IPs {
		label := ip.Address
		if ip.Netmask != "" {
			label += "/" + ip.Netmask
		}
		fmt.Fprintf(out, "%s: %s\n", ip.Family, label)
	}
	if iface.Gateway != "" {
		fmt.Fprintf(out, "gateway: %s\n", iface.Gateway)
	}
	if len(iface.DNS) > 0 {
		fmt.Fprintf(out, "dns:     %s\n", strings.Join(iface.DNS, ", "))
	}
	if iface.SSID != "" {
		fmt.Fprintf(out, "ssid:    %s\n", iface.SSID)
	}
}

func onOff(b bool) string {
	if b {
		return "up"
	}
	return "down"
}

func init() {
	rootCmd.AddCommand(listCmd, showCmd)
}
