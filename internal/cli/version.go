package cli

import "github.com/spf13/cobra"

// Version is set at build time via -ldflags.
var Version = "dev"

func newVersionCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print gantry version",
		RunE: func(cmd *cobra.Command, _ []string) error {
			cmd.Printf("gantry %s\n", Version)
			return nil
		},
	}
}
