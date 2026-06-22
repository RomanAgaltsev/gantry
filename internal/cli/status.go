package cli

import (
	"github.com/spf13/cobra"

	"github.com/RomanAgaltsev/gantry/internal/forge"
)

func newStatusCmd() *cobra.Command {
	var envName string
	cmd := &cobra.Command{
		Use:   "status",
		Short: "Show current pins vs. latest releases",
		RunE: func(cmd *cobra.Command, _ []string) error {
			d, err := buildDeps(cmd, envName, false)
			if err != nil {
				return err
			}
			env, _ := d.cfg.Environment(d.env)
			current, err := d.store.Read(env.PinFile)
			if err != nil {
				return err
			}
			for _, comp := range d.cfg.Components {
				rel, err := d.forge.LatestRelease(cmd.Context(), forge.Component{ID: comp.ID, Project: comp.Project, PinKey: comp.PinKey})
				if err != nil {
					return err
				}
				cmd.Printf("%-20s pinned=%-24s latest=%s\n", comp.PinKey, current[comp.PinKey], rel.ImageRef())
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&envName, "env", "", "environment name")
	_ = cmd.MarkFlagRequired("env")
	return cmd
}
