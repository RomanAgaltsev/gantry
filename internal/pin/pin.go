// Package pin models per-environment version-pin files (dotenv) and their diffs.
package pin

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"sort"
	"strings"
)

// Set maps a pin key to an immutable image reference.
type Set map[string]string

// Read parses a dotenv pin file.
func Read(r io.Reader) (Set, error) {
	s := Set{}
	sc := bufio.NewScanner(r)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		k, v, ok := strings.Cut(line, "=")
		if !ok {
			return nil, fmt.Errorf("malformed pin line: %q", line)
		}
		s[strings.TrimSpace(k)] = strings.TrimSpace(v)
	}
	return s, sc.Err()
}

// Render serializes a Set as dotenv with sorted keys.
func Render(s Set) []byte {
	keys := make([]string, 0, len(s))
	for k := range s {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var buf bytes.Buffer
	for _, k := range keys {
		fmt.Fprintf(&buf, "%s=%s\n", k, s[k])
	}
	return buf.Bytes()
}

// Change is a single pin difference.
type Change struct{ Key, Old, New string }

// Diff returns additions and changes from current to desired, sorted by key.
func Diff(current, desired Set) []Change {
	var out []Change
	keys := make([]string, 0, len(desired))
	for k := range desired {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		if cur, ok := current[k]; !ok || cur != desired[k] {
			out = append(out, Change{Key: k, Old: current[k], New: desired[k]})
		}
	}
	return out
}
