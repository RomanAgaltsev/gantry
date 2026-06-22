package cli

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/RomanAgaltsev/gantry/internal/config"
	"github.com/RomanAgaltsev/gantry/internal/forge"
	"github.com/RomanAgaltsev/gantry/internal/pin"
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
				line, err := componentStatusLine(cmd.Context(), comp, current, d.forge)
				if err != nil {
					return err
				}
				cmd.Println(line)
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&envName, "env", "", "environment name")
	_ = cmd.MarkFlagRequired("env")
	return cmd
}

func componentStatusLine(ctx context.Context, comp config.Component, current pin.Set, f forge.Forge) (string, error) {
	if comp.IsExplicit() {
		return fmt.Sprintf("%-20s pinned=%-24s latest=(untracked)", comp.PinKey, current[comp.PinKey]), nil
	}
	rel, err := f.LatestRelease(ctx, forge.Component{ID: comp.ID, Project: comp.Project, PinKey: comp.PinKey})
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%-20s pinned=%-24s latest=%s", comp.PinKey, current[comp.PinKey], rel.ImageRef()), nil
}
