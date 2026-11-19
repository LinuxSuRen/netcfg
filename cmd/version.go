package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

// versionCmd prints the semver injected at build time; `--version` keeps the
// same output via the root command's version template.
var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print the net-tool version",
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Fprintf(cmd.OutOrStdout(), "net-tool %s (commit %s)\n", version, commit)
	},
}

func init() {
	rootCmd.AddCommand(versionCmd)
}
