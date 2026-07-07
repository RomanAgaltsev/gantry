// Package humanize renders durations for gantry's user-facing strings (drift alarms in the
// CLI and the daemon), so the same event reads identically wherever it is emitted.
package humanize

import (
	"fmt"
	"time"
)

// Duration renders d as whole days when it is at least one day, otherwise whole hours.
func Duration(d time.Duration) string {
	if days := int(d.Hours()) / 24; days >= 1 {
		return fmt.Sprintf("%dd", days)
	}
	return fmt.Sprintf("%dh", int(d.Hours()))
}
