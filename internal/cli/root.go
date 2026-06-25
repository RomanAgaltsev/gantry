// Package cli wires gantry's cobra command tree to the engine.
package cli

import "github.com/spf13/cobra"

// NewRootCmd builds the gantry command tree.
func NewRootCmd() *cobra.Command {
	root := &cobra.Command{
		Use:           "gantry",
		Short:         "Non-Kubernetes release orchestrator",
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	root.PersistentFlags().String("config", "gantry.yaml", "path to gantry.yaml")
	root.AddCommand(
		newVersionCmd(),
		newSyncCmd(),
		newPlanCmd(),
		newStatusCmd(),
		newDeployCmd(),
		newPromoteCmd(),
		newRollbackCmd(),
		newHistoryCmd(),
	)

	return root
}

// Execute runs the root command.
func Execute() error { return NewRootCmd().Execute() }
