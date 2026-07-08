package cli

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/spf13/cobra"

	"github.com/RomanAgaltsev/gantry/internal/config"
	"github.com/RomanAgaltsev/gantry/internal/engine"
	"github.com/RomanAgaltsev/gantry/internal/forge"
	"github.com/RomanAgaltsev/gantry/internal/pin"
)

func newStatusCmd() *cobra.Command {
	var envName string
	var all, watch bool
	var interval time.Duration
	cmd := &cobra.Command{
		Use:   "status",
		Short: "Show current pins vs. latest releases (single env, or --all for the matrix)",
		RunE: func(cmd *cobra.Command, _ []string) error {
			if all && envName != "" {
				return errors.New("--all and --env are mutually exclusive")
			}
			if watch && outputIsJSON(cmd) {
				return errors.New("--watch is incompatible with --output json")
			}
			// --watch defaults to the cross-environment matrix when neither --all nor --env is set.
			if watch && !all && envName == "" {
				all = true
			}
			if !all && envName == "" {
				return errors.New("one of --env or --all is required")
			}
			if watch {
				return runStatusWatch(cmd, all, envName, interval)
			}
			if all {
				return runStatusAll(cmd)
			}
			return runStatusEnv(cmd, envName)
		},
	}
	cmd.Flags().StringVar(&envName, "env", "", "environment name")
	cmd.Flags().BoolVar(&all, "all", false, "show the cross-environment matrix")
	cmd.Flags().BoolVar(&watch, "watch", false, "refresh the status display until interrupted (Ctrl-C)")
	cmd.Flags().DurationVar(&interval, "interval", 5*time.Second, "refresh interval for --watch")
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

// clearScreen is the ANSI sequence used by --watch to reset the display before each
// refresh (cursor home + erase entire screen).
const clearScreen = "\033[H\033[2J"

// runStatusWatch refreshes the status display on an interval until the command's context is
// cancelled (Ctrl-C / signal). One refresh's error never exits the loop: it is printed to
// stderr and the next tick retries, because the operator most needs a live view during an
// incident where a refresh can transiently fail (review §9 item 18).
func runStatusWatch(cmd *cobra.Command, all bool, envName string, interval time.Duration) error {
	if interval <= 0 {
		interval = 5 * time.Second
	}
	t := time.NewTicker(interval)
	defer t.Stop()
	for {
		cmd.Print(clearScreen)
		var err error
		if all {
			err = runStatusAll(cmd)
		} else {
			err = runStatusEnv(cmd, envName)
		}
		if err != nil {
			cmd.PrintErrln(err)
		}
		select {
		case <-cmd.Context().Done():
			return nil
		case <-t.C:
		}
	}
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
