package cli

import (
	"github.com/spf13/cobra"

	"github.com/RomanAgaltsev/gantry/internal/engine"
)

func newRollbackCmd() *cobra.Command {
	var envName string
	var dryRun, fast bool
	cmd := &cobra.Command{
		Use:   "rollback",
		Short: "Roll an environment back to its previous pin set",
		RunE: func(cmd *cobra.Command, _ []string) error {
			release, err := acquireServeLock(cmd)
			if err != nil {
				return err
			}
			defer release()
			d, err := buildDeps(cmd, envName, false, !dryRun)
			if err != nil {
				return err
			}
			res, err := d.engine.Rollback(cmd.Context(), envName, d.exec, d.verify, engine.RollbackOptions{DryRun: dryRun, Fast: fast})
			d.notifier.Dispatch(cmd.Context(), rollbackEvents(envName, res, err)...)
			if err != nil {
				if hint := deployFailureHint(envName, res.Committed); hint != "" {
					cmd.PrintErrln(hint)
				}
				return err
			}
			if res.DryRun {
				if res.Slot != "" {
					cmd.Printf("would roll back %s by switching to %s\n", envName, res.Slot)
					return nil
				}
				cmd.Printf("would roll back %s to %.7s (%d pins)\n", envName, res.ToSHA, len(res.Pins))
				return nil
			}
			if fast {
				cmd.Printf("rolled back %s (fast) to release %s\n", envName, res.ToSHA)
				return nil
			}
			if res.Slot != "" {
				cmd.Printf("rolled back %s by switching to %s\n", envName, res.Slot)
				return nil
			}
			cmd.Printf("rolled back %s to %.7s; deployed %d pins\n", envName, res.ToSHA, len(res.Pins))
			return nil
		},
	}
	cmd.Flags().StringVar(&envName, "env", "", "environment name")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "show what would be rolled back without acting")
	cmd.Flags().BoolVar(&fast, "fast", false, "flip a symlink-release env to its previous release without pulling")
	mustRequireFlag(cmd, "env")
	return cmd
}
