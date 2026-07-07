package cli

import (
	"strings"

	"github.com/spf13/cobra"

	"github.com/RomanAgaltsev/gantry/internal/engine"
)

func newPlanCmd() *cobra.Command {
	var envName string
	cmd := &cobra.Command{
		Use:   "plan",
		Short: "Show pending pin changes (read-only)",
		RunE: func(cmd *cobra.Command, _ []string) error {
			d, err := buildDeps(cmd, envName, true, false)
			if err != nil {
				return err
			}
			res, err := engine.Sync(cmd.Context(), d.cfg, d.env, d.forge, d.exec, nil, d.store, d.ledger, engine.SyncOptions{DryRun: true})
			if err != nil {
				return err
			}
			printChanges(cmd, res.Changes, res.Deployed, res.Recovered)
			if env, ok := d.cfg.Environment(d.env); ok {
				current, rerr := d.store.Read(env.PinFile)
				if rerr != nil {
					return rerr
				}
				if orphans := engine.Orphans(d.cfg, current); len(orphans) > 0 {
					cmd.Printf("orphan pins (in pin file, not in config): %s — run `gantry prune --env %s`\n",
						strings.Join(orphans, ", "), d.env)
				}
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&envName, "env", "", "environment name")
	mustRequireFlag(cmd, "env")
	return cmd
}
