package cli

import (
	"errors"
	"time"

	"github.com/spf13/cobra"

	"github.com/RomanAgaltsev/gantry/internal/config"
	"github.com/RomanAgaltsev/gantry/internal/engine"
	"github.com/RomanAgaltsev/gantry/internal/humanize"
)

// ErrDriftDetected is returned by `gantry drift` when at least one component has
// drifted. It maps to exit code 3 (see ExitCode), distinct from operational errors.
var ErrDriftDetected = errors.New("drift detected")

// ExitCode maps a top-level command error to a process exit code:
// 0 = success, 3 = drift detected, 1 = any other (operational) error.
func ExitCode(err error) int {
	switch {
	case err == nil:
		return 0
	case errors.Is(err, ErrDriftDetected):
		return 3
	default:
		return 1
	}
}

func newDriftCmd() *cobra.Command {
	var envName string
	var all bool
	cmd := &cobra.Command{
		Use:   "drift",
		Short: "Report tracked components whose latest release is unconsumed past the threshold",
		RunE: func(cmd *cobra.Command, _ []string) error {
			envs, err := driftEnvs(cmd, envName, all)
			if err != nil {
				return err
			}
			if len(envs) == 0 {
				cmd.Println("no track-mode environments to check")
				return nil
			}
			drifted := false
			var items []engine.DriftItem
			for _, e := range envs {
				d, err := buildDeps(cmd, e, true, false)
				if err != nil {
					return err
				}
				rep, err := d.engine.Drift(cmd.Context(), e)
				if err != nil {
					return err
				}
				items = append(items, rep.Items...)
				if !outputIsJSON(cmd) {
					printDriftReport(cmd, rep, d.cfg.Drift.ThresholdOrDefault())
				}
				d.notifier.Dispatch(cmd.Context(), driftEvents(rep)...)
				drifted = drifted || rep.Drifted()
			}
			if outputIsJSON(cmd) {
				return printJSON(cmd, items)
			}
			if !drifted {
				cmd.Println("no drift")
				return nil
			}
			return ErrDriftDetected
		},
	}
	cmd.Flags().StringVar(&envName, "env", "", "environment name")
	cmd.Flags().BoolVar(&all, "all", false, "check every track-mode environment")
	cmd.MarkFlagsMutuallyExclusive("env", "all")
	cmd.MarkFlagsOneRequired("env", "all")
	return cmd
}

// driftEnvs resolves the environments to check. For --all it loads the config and
// returns every track-mode environment; for --env it returns the single name.
func driftEnvs(cmd *cobra.Command, envName string, all bool) ([]string, error) {
	if !all {
		return []string{envName}, nil
	}
	path, err := cmd.Flags().GetString("config")
	if err != nil {
		return nil, err
	}
	cfg, err := config.Load(path)
	if err != nil {
		return nil, err
	}
	var envs []string
	for _, env := range cfg.Environments {
		if env.Source.Track != "" {
			envs = append(envs, env.Name)
		}
	}
	return envs, nil
}

func printDriftReport(cmd *cobra.Command, rep engine.DriftReport, threshold time.Duration) {
	for _, it := range rep.Items {
		cmd.Printf("DRIFT %s/%s: pinned %s, latest %s published %s ago (>%s)\n",
			it.Env, it.Component, pinnedLabel(it.PinnedRef), it.Latest.SemverVersion,
			humanize.Duration(it.Age), humanize.Duration(threshold))
	}
}

func pinnedLabel(ref string) string {
	if ref == "" {
		return "(unpinned)"
	}
	return ref
}
