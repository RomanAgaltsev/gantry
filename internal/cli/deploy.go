package cli

import (
	"github.com/spf13/cobra"

	"github.com/RomanAgaltsev/gantry/internal/engine"
)

func newDeployCmd() *cobra.Command {
	var envName string
	cmd := &cobra.Command{
		Use:   "deploy",
		Short: "Reconcile an environment to its current committed pin file",
		RunE: func(cmd *cobra.Command, _ []string) error {
			d, err := buildDeps(cmd, envName, false, true)
			if err != nil {
				return err
			}
			res, err := engine.Deploy(cmd.Context(), d.cfg, d.env, d.exec, d.verify, d.store, d.ledger)
			if err != nil {
				return err
			}
			if res.Deployed {
				cmd.Printf("deployed %d pin(s) to %s\n", len(res.Pins), envName)
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&envName, "env", "", "environment name")
	mustRequireFlag(cmd, "env")
	return cmd
}
