// Package cli wires gantry's cobra command tree to the engine.
package cli

import (
	"github.com/spf13/cobra"

	"github.com/RomanAgaltsev/gantry/internal/logging"
)

// NewRootCmd builds the gantry command tree.
func NewRootCmd() *cobra.Command {
	root := &cobra.Command{
		Use:           "gantry",
		Short:         "Non-Kubernetes release orchestrator",
		SilenceUsage:  true,
		SilenceErrors: true,
		// PersistentPreRunE runs for every subcommand (unless it defines its own).
		// It builds the logger from the persistent flags and injects it into the
		// command context so the engine can emit diagnostics via logging.From(ctx).
		PersistentPreRunE: func(cmd *cobra.Command, _ []string) error {
			format, err := cmd.Flags().GetString("log-format")
			if err != nil {
				return err
			}
			level, err := cmd.Flags().GetString("log-level")
			if err != nil {
				return err
			}
			log := logging.New(format, level, cmd.ErrOrStderr())
			cmd.SetContext(logging.Into(cmd.Context(), log))
			return nil
		},
	}
	root.PersistentFlags().String("config", "gantry.yaml", "path to gantry.yaml")
	root.PersistentFlags().StringP("output", "o", "text", "output format: text|json")
	root.PersistentFlags().String("log-format", "text", "log format: text|json")
	root.PersistentFlags().String("log-level", "info", "log level: debug|info|warn|error")
	root.AddCommand(
		newVersionCmd(),
		newSyncCmd(),
		newPlanCmd(),
		newStatusCmd(),
		newDeployCmd(),
		newPromoteCmd(),
		newRollbackCmd(),
		newPruneCmd(),
		newHistoryCmd(),
		newDriftCmd(),
		newDiffCmd(),
		newSwitchCmd(),
		newServeCmd(),
	)

	return root
}

// Execute runs the root command.
func Execute() error { return NewRootCmd().Execute() }
