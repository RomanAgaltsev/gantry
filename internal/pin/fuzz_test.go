package pin

import (
	"strings"
	"testing"
)

// FuzzRead asserts the dotenv parser never panics on arbitrary input and that a successfully
// parsed set round-trips: rendering it and re-reading yields the same number of keys.
func FuzzRead(f *testing.F) {
	f.Add("A=1\nB=2\n")
	f.Add("# comment\n\nKEY=val\n")
	f.Add("MALFORMED\n")
	f.Add("  spaced  =  val  \n")
	f.Fuzz(func(t *testing.T, s string) {
		set, err := Read(strings.NewReader(s))
		if err != nil {
			return // errors are expected on arbitrary input; only a panic would be a bug
		}
		again, err := Read(strings.NewReader(string(Render(set))))
		if err != nil {
			t.Fatalf("re-parse of rendered set failed: %v", err)
		}
		if len(again) != len(set) {
			t.Fatalf("round-trip changed key count: %d -> %d", len(set), len(again))
		}
	})
}
