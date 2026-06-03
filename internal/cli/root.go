package cli

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:           "resolved",
	Short:         "Find stale GitHub issue/PR references in code comments",
	SilenceUsage:  true,
	SilenceErrors: true,
}

// Execute runs the root command. Errors exit with code 2 (tool error);
// subcommands set more specific exit codes via os.Exit themselves.
func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, "resolved:", err)
		os.Exit(2)
	}
}
