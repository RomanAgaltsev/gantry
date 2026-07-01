package config

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// defaultDriftThreshold is used when drift.threshold is unset.
const defaultDriftThreshold = 7 * 24 * time.Hour

// Duration is a time.Duration that unmarshals from YAML, additionally accepting a
// whole-day "d" unit (e.g. "7d" = 168h) that time.ParseDuration does not support.
type Duration time.Duration

// UnmarshalYAML parses a duration string, accepting "<int>d" for whole days.
func (d *Duration) UnmarshalYAML(value *yaml.Node) error {
	var s string
	if err := value.Decode(&s); err != nil {
		return err
	}
	dur, err := parseDuration(s)
	if err != nil {
		return err
	}
	*d = Duration(dur)
	return nil
}

func parseDuration(s string) (time.Duration, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, errors.New("empty duration")
	}
	if days, ok := strings.CutSuffix(s, "d"); ok {
		n, err := strconv.Atoi(days)
		if err != nil {
			return 0, fmt.Errorf("invalid day duration %q: %w", s, err)
		}
		return time.Duration(n) * 24 * time.Hour, nil
	}
	return time.ParseDuration(s)
}

// DriftConfig tunes the drift detector. The block is optional.
type DriftConfig struct {
	Threshold Duration `yaml:"threshold"`
}

// ThresholdOrDefault returns the configured threshold, or 7d when unset.
func (c DriftConfig) ThresholdOrDefault() time.Duration {
	if c.Threshold == 0 {
		return defaultDriftThreshold
	}
	return time.Duration(c.Threshold)
}
