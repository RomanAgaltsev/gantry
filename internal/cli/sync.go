package cli

import (
	"github.com/spf13/cobra"

	"github.com/RomanAgaltsev/gantry/internal/engine"
	"github.com/RomanAgaltsev/gantry/internal/pin"
)

func newSyncCmd() *cobra.Command {
	var envName string
	var dryRun bool
	cmd := &cobra.Command{
		Use:   "sync",
		Short: "Consume releases, pin, and deploy an environment",
		RunE: func(cmd *cobra.Command, _ []string) error {
			release, err := acquireServeLock(cmd)
			if err != nil {
				return err
			}
			defer release()
			d, err := buildDeps(cmd, envName, true, !dryRun)
			if err != nil {
				return err
			}
			res, err := d.engine.Sync(cmd.Context(), d.env, d.exec, d.verify, engine.SyncOptions{DryRun: dryRun})
			d.notifier.Dispatch(cmd.Context(), syncEvents(envName, res, err)...)
			if err != nil {
				if note := autoRollbackNote(envName, res.RolledBackTo); note != "" {
					cmd.PrintErrln(note)
				}
				return err
			}
			printChanges(cmd, res.Changes, res.Deployed, res.Recovered)
			return nil
		},
	}
	cmd.Flags().StringVar(&envName, "env", "", "environment name")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "show changes without writing/deploying")
	mustRequireFlag(cmd, "env")
	return cmd
}

func printChanges(cmd *cobra.Command, changes []pin.Change, deployed, recovered bool) {
	if len(changes) == 0 {
		cmd.Println(upToDateMessage(deployed, recovered))
		return
	}
	for _, c := range changes {
		cmd.Printf("%s: %s -> %s\n", c.Key, c.Old, c.New)
	}
	if deployed {
		cmd.Println("deployed")
	}
}

// upToDateMessage renders the no-changes line for sync/plan. A recovery with no deploy is
// a dry run (plan, or sync --dry-run): it must read as a prediction, not a past action.
func upToDateMessage(deployed, recovered bool) string {
	switch {
	case recovered && deployed:
		return "recovered: redeployed the last committed pin set"
	case recovered:
		return "would redeploy the last committed pin set (not yet green)"
	default:
		return "up to date; no changes"
	}
}
