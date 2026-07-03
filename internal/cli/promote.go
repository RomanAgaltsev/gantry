package cli

import (
	"github.com/spf13/cobra"

	"github.com/RomanAgaltsev/gantry/internal/engine"
)

func newPromoteCmd() *cobra.Command {
	var fromEnv, toEnv, sha string
	var dryRun bool
	cmd := &cobra.Command{
		Use:   "promote",
		Short: "Promote a verified pin set from one environment to another",
		RunE: func(cmd *cobra.Command, _ []string) error {
			if err := guardServe(cmd); err != nil {
				return err
			}
			d, err := buildDeps(cmd, toEnv, false, !dryRun)
			if err != nil {
				return err
			}
			if env, ok := d.cfg.Environment(toEnv); ok {
				if w := promoteDAGWarning(toEnv, env.Source.PromoteFrom, fromEnv); w != "" {
					cmd.PrintErrln(w)
				}
			}
			res, err := engine.Promote(cmd.Context(), d.cfg, fromEnv, toEnv, sha, d.exec, d.verify, d.store, d.ledger, engine.PromoteOptions{DryRun: dryRun})
			d.notifier.Dispatch(cmd.Context(), promoteEvents(fromEnv, toEnv, res, err)...)
			if err != nil {
				if note := autoRollbackNote(toEnv, res.RolledBackTo); note != "" {
					cmd.PrintErrln(note)
				} else if hint := deployFailureHint(toEnv, res.Committed); hint != "" {
					cmd.PrintErrln(hint)
				}
				return err
			}
			if res.DryRun {
				cmd.Printf("would promote %s@%.7s -> %s (%d pins)\n", fromEnv, res.FromSHA, toEnv, len(res.Pins))
				return nil
			}
			cmd.Printf("promoted %s@%.7s -> %s; deployed %d pins\n", fromEnv, res.FromSHA, toEnv, len(res.Pins))
			return nil
		},
	}
	cmd.Flags().StringVar(&fromEnv, "from", "", "source environment")
	cmd.Flags().StringVar(&toEnv, "to", "", "target environment")
	cmd.Flags().StringVar(&sha, "sha", "", "source pin commit (default: latest green)")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "show what would be promoted without acting")
	mustRequireFlag(cmd, "from")
	mustRequireFlag(cmd, "to")
	return cmd
}
