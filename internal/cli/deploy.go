package cli

import (
	"github.com/spf13/cobra"
)

func newDeployCmd() *cobra.Command {
	var envName string
	cmd := &cobra.Command{
		Use:   "deploy",
		Short: "Reconcile an environment to its current committed pin file",
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
			res, err := d.engine.Deploy(cmd.Context(), d.env, d.exec, d.verify)
			d.notifier.Dispatch(cmd.Context(), deployEvents(envName, res, err)...)
			if err != nil {
				if note := autoRollbackNote(envName, res.RolledBackTo); note != "" {
					cmd.PrintErrln(note)
				}
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
