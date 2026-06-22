package cli

import (
	"github.com/spf13/cobra"

	"github.com/RomanAgaltsev/gantry/internal/engine"
)

func newPlanCmd() *cobra.Command {
	var envName string
	cmd := &cobra.Command{
		Use:   "plan",
		Short: "Show pending pin changes (read-only)",
		RunE: func(cmd *cobra.Command, _ []string) error {
			d, err := buildDeps(cmd, envName, false)
			if err != nil {
				return err
			}
			res, err := engine.Sync(cmd.Context(), d.cfg, d.env, d.forge, d.exec, d.store, engine.SyncOptions{DryRun: true})
			if err != nil {
				return err
			}
			printChanges(cmd, res.Changes, false)
			return nil
		},
	}
	cmd.Flags().StringVar(&envName, "env", "", "environment name")
	_ = cmd.MarkFlagRequired("env")
	return cmd
}
