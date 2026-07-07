package cli

import (
	"context"
	"errors"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/RomanAgaltsev/gantry/internal/config"
	"github.com/RomanAgaltsev/gantry/internal/engine"
	"github.com/RomanAgaltsev/gantry/internal/forge"
	"github.com/RomanAgaltsev/gantry/internal/pin"
)

func newStatusCmd() *cobra.Command {
	var envName string
	var all bool
	cmd := &cobra.Command{
		Use:   "status",
		Short: "Show current pins vs. latest releases (single env, or --all for the matrix)",
		RunE: func(cmd *cobra.Command, _ []string) error {
			if all && envName != "" {
				return errors.New("--all and --env are mutually exclusive")
			}
			if !all && envName == "" {
				return errors.New("one of --env or --all is required")
			}
			if all {
				return runStatusAll(cmd)
			}
			return runStatusEnv(cmd, envName)
		},
	}
	cmd.Flags().StringVar(&envName, "env", "", "environment name")
	cmd.Flags().BoolVar(&all, "all", false, "show the cross-environment matrix")
	return cmd
}

// runStatusAll prints the cross-environment matrix.
func runStatusAll(cmd *cobra.Command) error {
	d, err := buildDeps(cmd, "", true, false)
	if err != nil {
		return err
	}
	m, err := d.engine.StatusMatrix(cmd.Context())
	if err != nil {
		return err
	}
	cmd.Print(engine.FormatMatrix(m))
	return nil
}

// runStatusEnv prints the single-environment pin-vs-latest list (the original behavior).
func runStatusEnv(cmd *cobra.Command, envName string) error {
	d, err := buildDeps(cmd, envName, true, false)
	if err != nil {
		return err
	}
	env, _ := d.cfg.Environment(d.env)
	current, err := d.engine.Store.Read(cmd.Context(), env.PinFile)
	if err != nil {
		return err
	}
	for _, comp := range d.cfg.Components {
		cmd.Println(componentStatusLine(cmd.Context(), comp, current, d.engine.Forge))
	}
	return nil
}

func componentStatusLine(ctx context.Context, comp config.Component, current pin.Set, f forge.Forge) string {
	if comp.IsExplicit() {
		return fmt.Sprintf("%-20s pinned=%-24s latest=(untracked)", comp.PinKey, current[comp.PinKey])
	}
	rel, err := f.LatestRelease(ctx, forge.Component{ID: comp.ID, Project: comp.Project, PinKey: comp.PinKey})
	if err != nil {
		// A forge blip for one component degrades that line to latest=(error) instead of
		// failing the whole status output — you most need status during an incident (C5).
		return fmt.Sprintf("%-20s pinned=%-24s latest=(error)", comp.PinKey, current[comp.PinKey])
	}
	return fmt.Sprintf("%-20s pinned=%-24s latest=%s", comp.PinKey, current[comp.PinKey], rel.ImageRef())
}
