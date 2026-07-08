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
	if outputIsJSON(cmd) {
		return printJSON(cmd, m)
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
	if outputIsJSON(cmd) {
		rows := make([]envStatusRow, 0, len(d.cfg.Components))
		for _, comp := range d.cfg.Components {
			rows = append(rows, envStatusRowFor(cmd.Context(), comp, current, d.engine.Forge))
		}
		return printJSON(cmd, rows)
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

// envStatusRow is one component's pin-vs-latest cell in the JSON output of `status --env`.
// It mirrors the text componentStatusLine, including the "(untracked)"/"(error)" sentinels.
type envStatusRow struct {
	PinKey    string `json:"pin_key"`
	Pinned    string `json:"pinned"`
	Latest    string `json:"latest"`
	Untracked bool   `json:"untracked"` // true for explicit-pin components (no gantry-known latest)
	Error     bool   `json:"error"`     // true when the latest-release fetch failed
}

// envStatusRowFor builds one JSON row for a component, degrading a forge error to an
// error-flagged cell instead of failing the whole status output (C5). The Latest sentinels
// match the text output's "(untracked)"/"(error)" strings (see componentStatusLine).
func envStatusRowFor(ctx context.Context, comp config.Component, current pin.Set, f forge.Forge) envStatusRow {
	if comp.IsExplicit() {
		return envStatusRow{PinKey: comp.PinKey, Pinned: current[comp.PinKey], Latest: "(untracked)", Untracked: true}
	}
	rel, err := f.LatestRelease(ctx, forge.Component{ID: comp.ID, Project: comp.Project, PinKey: comp.PinKey})
	if err != nil {
		return envStatusRow{PinKey: comp.PinKey, Pinned: current[comp.PinKey], Latest: "(error)", Error: true}
	}
	return envStatusRow{PinKey: comp.PinKey, Pinned: current[comp.PinKey], Latest: rel.ImageRef()}
}
