package cli

import (
	"github.com/spf13/cobra"

	"github.com/RomanAgaltsev/gantry/internal/engine"
)

func newSwitchCmd() *cobra.Command {
	var envName string
	cmd := &cobra.Command{
		Use:   "switch",
		Short: "Promote a blue-green environment's idle slot by switching the pointer",
		RunE: func(cmd *cobra.Command, _ []string) error {
			release, err := acquireServeLock(cmd)
			if err != nil {
				return err
			}
			defer release()
			d, err := buildDeps(cmd, envName, false, true)
			if err != nil {
				return err
			}
			res, err := engine.Switch(cmd.Context(), d.cfg, envName, d.exec, d.verify, d.store, d.ledger)
			if err != nil {
				return err
			}
			from := res.From
			if from == "" {
				from = "(none)"
			}
			cmd.Printf("switched %s: %s -> %s\n", envName, from, res.To)
			return nil
		},
	}
	cmd.Flags().StringVar(&envName, "env", "", "environment name")
	mustRequireFlag(cmd, "env")
	return cmd
}
