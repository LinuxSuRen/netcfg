package cmd

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/LinuxSuRen/netcfg/internal/netconf"
)

var (
	setIPDry     bool
	setIPDHCP    bool
	setIPIP      string
	setIPMask    string
	setIPGateway string
)

var setIPCmd = &cobra.Command{
	Use:   "set-ip <interface>",
	Short: "Switch an interface to DHCP or a static IPv4 address",
	Long: `Switch an interface to DHCP or a static IPv4 address.

Static form needs --ip and --mask; --mask defaults to 255.255.255.0 and
--gw is optional. Requires administrator privileges; --dry-run prints the
exact system commands instead of running them.`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg := netconf.IPConfig{DHCP: setIPDHCP, IP: setIPIP, Netmask: setIPMask, Gateway: setIPGateway}
		if cfg.DHCP {
			if cfg.IP != "" || cfg.Netmask != "" || cfg.Gateway != "" {
				return exitErr(cmd, fmt.Errorf("--dhcp cannot be combined with --ip/--mask/--gw"))
			}
		} else {
			if cfg.IP == "" {
				return exitErr(cmd, fmt.Errorf("--ip is required (or pass --dhcp)"))
			}
			if cfg.Netmask == "" {
				cfg.Netmask = "255.255.255.0"
			}
		}
		mgr := newManager(setIPDry)
		if err := mgr.SetIP(args[0], cfg); err != nil {
			return exitErr(cmd, err)
		}
		if setIPDry {
			reportDry(mgr)
			return nil
		}
		if cfg.DHCP {
			fmt.Fprintf(cmd.OutOrStdout(), "%s switched to DHCP\n", args[0])
		} else {
			fmt.Fprintf(cmd.OutOrStdout(), "%s set to %s/%s", args[0], cfg.IP, cfg.Netmask)
			if cfg.Gateway != "" {
				fmt.Fprintf(cmd.OutOrStdout(), " gw %s", cfg.Gateway)
			}
			fmt.Fprintln(cmd.OutOrStdout())
		}
		return nil
	},
}

var (
	setDNSDry     bool
	setDNSAuto    bool
	setDNSServers []string
)

var setDNSCmd = &cobra.Command{
	Use:   "set-dns <interface>",
	Short: "Set manual DNS servers or return an interface to automatic DNS",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if !setDNSAuto && len(setDNSServers) == 0 {
			return exitErr(cmd, fmt.Errorf("pass --servers or --auto"))
		}
		cfg := netconf.DNSConfig{Auto: setDNSAuto, Servers: setDNSServers}
		mgr := newManager(setDNSDry)
		if err := mgr.SetDNS(args[0], cfg); err != nil {
			return exitErr(cmd, err)
		}
		if setDNSDry {
			reportDry(mgr)
			return nil
		}
		if setDNSAuto {
			fmt.Fprintf(cmd.OutOrStdout(), "%s back to automatic DNS\n", args[0])
		} else {
			fmt.Fprintf(cmd.OutOrStdout(), "%s DNS set to %v\n", args[0], setDNSServers)
		}
		return nil
	},
}

func init() {
	f := setIPCmd.Flags()
	f.BoolVar(&setIPDHCP, "dhcp", false, "use DHCP instead of a static address")
	f.StringVar(&setIPIP, "ip", "", "static IPv4 address")
	f.StringVar(&setIPMask, "mask", "", "subnet mask (default 255.255.255.0)")
	f.StringVar(&setIPGateway, "gw", "", "default gateway")
	f.BoolVar(&setIPDry, "dry-run", false, "print the system commands instead of running them")
	rootCmd.AddCommand(setIPCmd)

	f = setDNSCmd.Flags()
	f.StringSliceVar(&setDNSServers, "servers", nil, "comma-separated DNS servers")
	f.BoolVar(&setDNSAuto, "auto", false, "clear manual servers, use DHCP-provided DNS")
	f.BoolVar(&setDNSDry, "dry-run", false, "print the system commands instead of running them")
	rootCmd.AddCommand(setDNSCmd)
}
