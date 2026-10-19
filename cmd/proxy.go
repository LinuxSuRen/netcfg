package cmd

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/LinuxSuRen/netcfg/internal/netconf"
)

var (
	proxyIface  string
	proxyHTTP   string
	proxyHTTPS  string
	proxyBypass []string
	proxyDry    bool
)

var proxyGetCmd = &cobra.Command{
	Use:   "get",
	Short: "Show the current system proxy",
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := netconf.NewManager().Proxy(proxyIface)
		if err != nil {
			return exitErr(cmd, err)
		}
		if outputJSON {
			printJSON(cmd.OutOrStdout(), cfg)
			return nil
		}
		printProxy(cmd, cfg)
		return nil
	},
}

var proxySetCmd = &cobra.Command{
	Use:   "set",
	Short: "Set a manual system proxy (--http host:port)",
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := proxyConfigFromFlags()
		if err != nil {
			return exitErr(cmd, err)
		}
		mgr := newManager(proxyDry)
		if err := mgr.SetProxy(proxyIface, cfg); err != nil {
			return exitErr(cmd, err)
		}
		if proxyDry {
			reportDry(mgr)
			return nil
		}
		fmt.Fprintf(cmd.OutOrStdout(), "proxy set\n")
		return nil
	},
}

var proxyOffCmd = &cobra.Command{
	Use:   "off",
	Short: "Disable the system proxy",
	RunE: func(cmd *cobra.Command, args []string) error {
		mgr := newManager(proxyDry)
		if err := mgr.SetProxy(proxyIface, netconf.ProxyConfig{Mode: "none"}); err != nil {
			return exitErr(cmd, err)
		}
		if proxyDry {
			reportDry(mgr)
			return nil
		}
		fmt.Fprintf(cmd.OutOrStdout(), "proxy off\n")
		return nil
	},
}

func proxyConfigFromFlags() (netconf.ProxyConfig, error) {
	cfg := netconf.ProxyConfig{Mode: "manual", Bypass: proxyBypass}
	if proxyHTTP != "" {
		host, port, err := splitHostPort(proxyHTTP)
		if err != nil {
			return cfg, fmt.Errorf("--http: %v", err)
		}
		cfg.HTTPHost, cfg.HTTPPort = host, port
	}
	if proxyHTTPS != "" {
		host, port, err := splitHostPort(proxyHTTPS)
		if err != nil {
			return cfg, fmt.Errorf("--https: %v", err)
		}
		cfg.HTTPSHost, cfg.HTTPSPort = host, port
	}
	if cfg.HTTPHost == "" && cfg.HTTPSHost == "" {
		return cfg, fmt.Errorf("pass --http host:port (and optionally --https)")
	}
	if cfg.HTTPSHost == "" {
		cfg.HTTPSHost, cfg.HTTPSPort = cfg.HTTPHost, cfg.HTTPPort
	}
	return cfg, nil
}

func splitHostPort(s string) (string, int, error) {
	host, portStr, ok := strings.Cut(s, ":")
	if !ok || host == "" {
		return "", 0, fmt.Errorf("%q is not host:port", s)
	}
	var port int
	if _, err := fmt.Sscanf(portStr, "%d", &port); err != nil || port <= 0 || port > 65535 {
		return "", 0, fmt.Errorf("port %q is invalid", portStr)
	}
	return host, port, nil
}

func printProxy(cmd *cobra.Command, cfg netconf.ProxyConfig) {
	out := cmd.OutOrStdout()
	if cfg.Mode == "none" {
		fmt.Fprintln(out, "mode: none")
		return
	}
	fmt.Fprintln(out, "mode: manual")
	if cfg.HTTPHost != "" {
		fmt.Fprintf(out, "http:  %s:%d\n", cfg.HTTPHost, cfg.HTTPPort)
	}
	if cfg.HTTPSHost != "" {
		fmt.Fprintf(out, "https: %s:%d\n", cfg.HTTPSHost, cfg.HTTPSPort)
	}
	if len(cfg.Bypass) > 0 {
		fmt.Fprintf(out, "bypass: %s\n", strings.Join(cfg.Bypass, ", "))
	}
}

var proxyCmd = &cobra.Command{
	Use:   "proxy",
	Short: "Read and set the system proxy",
	Long: `Read and set the system proxy.

On macOS the proxy belongs to a network service (see "netcfg list"); pass
--iface to pick one, otherwise the Wi-Fi service is used. On Windows and
Linux the setting is machine/session-wide.`,
}

func init() {
	proxyCmd.PersistentFlags().StringVar(&proxyIface, "iface", "", "network service/interface the proxy applies to")
	f := proxySetCmd.Flags()
	f.StringVar(&proxyHTTP, "http", "", "HTTP proxy as host:port")
	f.StringVar(&proxyHTTPS, "https", "", "HTTPS proxy as host:port (defaults to --http)")
	f.StringSliceVar(&proxyBypass, "bypass", nil, "comma-separated proxy bypass list")
	f.BoolVar(&proxyDry, "dry-run", false, "print the system commands instead of running them")
	proxyOffCmd.Flags().BoolVar(&proxyDry, "dry-run", false, "print the system commands instead of running them")
	proxyCmd.AddCommand(proxyGetCmd, proxySetCmd, proxyOffCmd)
	rootCmd.AddCommand(proxyCmd)
}
