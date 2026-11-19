package cmd

import (
	"fmt"
	"net"
	"net/http"

	"github.com/spf13/cobra"

	"github.com/LinuxSuRen/net-tool/internal/api"
	"github.com/LinuxSuRen/net-tool/internal/netconf"
	"github.com/LinuxSuRen/net-tool/web"
)

var serveAddr string

var serveCmd = &cobra.Command{
	Use:   "serve",
	Short: "Serve the Web UI and REST API on localhost",
	Long: `Serve the Web UI and REST API.

Listens on 127.0.0.1:8190 by default. The server never binds a public
interface unless you pass --addr yourself; changing network configuration
is a privileged operation, so keep it local (or tunnel it securely) and run
net-tool with the privileges the operations need.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		api.Version, api.Commit = version, commit
		srv := api.New(netconf.NewManager(), netconf.NewHosts(), web.Dist())
		listener, err := net.Listen("tcp", serveAddr)
		if err != nil {
			return exitErr(cmd, err)
		}
		addr := listener.Addr().String()
		fmt.Fprintf(cmd.OutOrStdout(), "net-tool web UI: http://%s\n", addr)
		if !isLoopbackAddr(addr) {
			fmt.Fprintln(cmd.ErrOrStderr(), "warning: bound on a non-loopback address; anyone reachable can change this machine's network settings")
		}
		return http.Serve(listener, srv.Handler())
	},
}

// isLoopbackAddr reports whether host:port points at localhost.
func isLoopbackAddr(addr string) bool {
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		return false
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

func init() {
	serveCmd.Flags().StringVar(&serveAddr, "addr", "127.0.0.1:8190", "listen address")
	rootCmd.AddCommand(serveCmd)
}
