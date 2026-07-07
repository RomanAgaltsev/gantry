package cli

import (
	"github.com/spf13/cobra"
)

func newHistoryCmd() *cobra.Command {
	var envName string
	cmd := &cobra.Command{
		Use:   "history",
		Short: "Show the deploy-outcome ledger for an environment (newest first)",
		RunE: func(cmd *cobra.Command, _ []string) error {
			d, err := buildDeps(cmd, envName, false, false) // read-only: no forge or executor needed
			if err != nil {
				return err
			}
			entries, err := d.engine.Ledger.History(cmd.Context(), envName)
			if err != nil {
				return err
			}
			if len(entries) == 0 {
				cmd.Printf("no deploy history for %s\n", envName)
				return nil
			}
			for _, e := range entries {
				cmd.Printf("%s  %-7s  %-8s  healthy=%-7s  by=%s  %.7s\n",
					e.DeployedAt.Format("2006-01-02T15:04:05Z07:00"), e.Result, e.Environment, e.Healthy, e.By, e.PinCommit)
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&envName, "env", "", "environment name")
	mustRequireFlag(cmd, "env")
	return cmd
}
