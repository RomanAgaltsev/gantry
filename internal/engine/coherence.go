package engine

import (
	"sort"

	"github.com/RomanAgaltsev/gantry/internal/config"
	"github.com/RomanAgaltsev/gantry/internal/pin"
)

// configPinKeys returns the set of pin keys declared by the config's components.
func configPinKeys(cfg *config.Config) map[string]bool {
	keys := make(map[string]bool, len(cfg.Components))
	for _, c := range cfg.Components {
		keys[c.PinKey] = true
	}
	return keys
}

// Orphans returns the pin keys present in current but backed by no config component — pins
// left behind when a component was deleted or its pin_key renamed (review D2). Sorted.
func Orphans(cfg *config.Config, current pin.Set) []string {
	known := configPinKeys(cfg)
	var out []string
	for k := range current {
		if !known[k] {
			out = append(out, k)
		}
	}
	sort.Strings(out)
	return out
}

// MissingKeys returns the config pin_keys absent from current — a declared component whose pin
// was never written, which would deploy an empty image reference (review D3). Sorted.
func MissingKeys(cfg *config.Config, current pin.Set) []string {
	var out []string
	for _, c := range cfg.Components {
		if _, ok := current[c.PinKey]; !ok {
			out = append(out, c.PinKey)
		}
	}
	sort.Strings(out)
	return out
}
