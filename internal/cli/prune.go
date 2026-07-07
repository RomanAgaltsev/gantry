package cli

import (
	"github.com/spf13/cobra"

	"github.com/RomanAgaltsev/gantry/internal/engine"
)

func newPruneCmd() *cobra.Command {
	var envName string
	var dryRun bool
	cmd := &cobra.Command{
		Use:   "prune",
		Short: "Remove pin keys no longer backed by a config component, then redeploy",
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
			res, err := engine.Prune(cmd.Context(), d.cfg, d.env, d.exec, d.verify, d.store, d.ledger, engine.PruneOptions{DryRun: dryRun})
			if err != nil {
				return err
			}
			if len(res.Removed) == 0 {
				cmd.Println("no orphan pins to prune")
				return nil
			}
			for _, k := range res.Removed {
				cmd.Printf("removed %s\n", k)
			}
			if res.Deployed {
				cmd.Println("redeployed")
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&envName, "env", "", "environment name")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "show orphans without writing/deploying")
	mustRequireFlag(cmd, "env")
	return cmd
}
