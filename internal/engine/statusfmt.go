package engine

import (
	"fmt"
	"strings"
	"time"
)

// FormatMatrix renders a Matrix as an aligned text table plus a per-environment
// health footer. Pure: same Matrix in, same string out. Output ends with a newline.
func FormatMatrix(m Matrix) string {
	headers := append([]string{"COMPONENT", "latest"}, m.Environments...)

	// Build each row's cells (component, latest, then one per environment).
	rows := make([][]string, 0, len(m.Components))
	for _, k := range m.Components {
		row := append([]string(nil), k, m.Latest[k])
		for _, env := range m.Environments {
			cell := m.Pins[env][k]
			if m.Drift[env][k] {
				cell += " !"
			}
			row = append(row, cell)
		}
		rows = append(rows, row)
	}

	// Column widths from headers and all cells.
	widths := make([]int, len(headers))
	for i, h := range headers {
		widths[i] = len(h)
	}
	for _, row := range rows {
		for i, cell := range row {
			if len(cell) > widths[i] {
				widths[i] = len(cell)
			}
		}
	}

	var b strings.Builder
	writeRow(&b, headers, widths)
	for _, row := range rows {
		writeRow(&b, row, widths)
	}

	// Health footer.
	b.WriteString("\n")
	for _, h := range m.Health {
		if !h.HasData {
			fmt.Fprintf(&b, "%s  (no deploys)\n", h.Env)
			continue
		}
		fmt.Fprintf(&b, "%s  %-7s  %-9s  %s ago\n", h.Env, h.Result, healthLabel(h.Healthy), humanizeAge(h.Age))
	}
	return b.String()
}

// writeRow writes one left-aligned, space-padded row, trimming trailing spaces.
func writeRow(b *strings.Builder, cells []string, widths []int) {
	parts := make([]string, len(cells))
	for i, c := range cells {
		parts[i] = fmt.Sprintf("%-*s", widths[i], c)
	}
	b.WriteString(strings.TrimRight(strings.Join(parts, "  "), " "))
	b.WriteString("\n")
}

// healthLabel renders a ledger Healthy value for the footer.
func healthLabel(h string) string {
	switch h {
	case "true":
		return "healthy"
	case "false":
		return "unhealthy"
	default:
		return h // "unknown" or empty
	}
}

// humanizeAge renders a duration compactly: minutes under an hour, hours under a
// day, otherwise days.
func humanizeAge(d time.Duration) string {
	switch {
	case d < time.Hour:
		return fmt.Sprintf("%dm", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh", int(d.Hours()))
	default:
		return fmt.Sprintf("%dd", int(d.Hours())/24)
	}
}
