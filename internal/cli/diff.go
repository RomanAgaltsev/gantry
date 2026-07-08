package cli

import (
	"fmt"
	"sort"

	"github.com/spf13/cobra"

	"github.com/RomanAgaltsev/gantry/internal/pin"
)

func newDiffCmd() *cobra.Command {
	var envName, to string
	cmd := &cobra.Command{
		Use:   "diff",
		Short: "Show pin differences between two environments",
		RunE: func(cmd *cobra.Command, _ []string) error {
			d, err := buildDeps(cmd, "", false, false) // read-only, no forge/executor needed
			if err != nil {
				return err
			}
			ea, ok := d.cfg.Environment(envName)
			if !ok {
				return fmt.Errorf("environment %q not found", envName)
			}
			eb, ok := d.cfg.Environment(to)
			if !ok {
				return fmt.Errorf("environment %q not found", to)
			}
			pa, err := d.engine.Store.Read(cmd.Context(), ea.PinFile)
			if err != nil {
				return err
			}
			pb, err := d.engine.Store.Read(cmd.Context(), eb.PinFile)
			if err != nil {
				return err
			}

			diffs := pinDiffs(pa, pb)
			if outputIsJSON(cmd) {
				return printJSON(cmd, diffs)
			}
			if len(diffs) == 0 {
				cmd.Printf("%s and %s are identical\n", envName, to)
				return nil
			}
			for _, r := range diffs {
				cmd.Printf("%-20s %s=%s  %s=%s\n", r.Key, envName, orDash(r.A), to, orDash(r.B))
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&envName, "env", "", "first environment")
	cmd.Flags().StringVar(&to, "to", "", "second environment")
	mustRequireFlag(cmd, "env")
	mustRequireFlag(cmd, "to")
	return cmd
}

// diffRow is one differing pin between two environments, for the text and JSON outputs.
type diffRow struct {
	Key string `json:"key"`
	A   string `json:"a"`
	B   string `json:"b"`
}

// pinDiffs returns the pins that differ between pa and pb, sorted by key. Each row carries
// the value under A (pa) and B (pb); an absent pin renders as "" (shown as "-" in text).
func pinDiffs(pa, pb pin.Set) []diffRow {
	keys := unionKeys(pa, pb)
	sort.Strings(keys)
	var diffs []diffRow
	for _, k := range keys {
		if pa[k] != pb[k] {
			diffs = append(diffs, diffRow{Key: k, A: pa[k], B: pb[k]})
		}
	}
	return diffs
}

// unionKeys returns the sorted union of the keys of the two pin sets.
func unionKeys(pa, pb pin.Set) []string {
	seen := make(map[string]struct{}, len(pa)+len(pb))
	for k := range pa {
		seen[k] = struct{}{}
	}
	for k := range pb {
		seen[k] = struct{}{}
	}
	keys := make([]string, 0, len(seen))
	for k := range seen {
		keys = append(keys, k)
	}
	return keys
}

// orDash renders an empty pin as "-" so a missing pin is visible in the text diff.
func orDash(s string) string {
	if s == "" {
		return "-"
	}
	return s
}
