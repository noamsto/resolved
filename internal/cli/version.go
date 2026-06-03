package cli

import "github.com/spf13/cobra"

// version is overridden at build time via -ldflags "-X ...cli.version=...".
var version = "dev"

func init() {
	rootCmd.AddCommand(&cobra.Command{
		Use:   "version",
		Short: "Print the version",
		Run: func(cmd *cobra.Command, _ []string) {
			cmd.Println(version)
		},
	})
}
