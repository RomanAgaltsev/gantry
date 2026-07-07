package composessh

import (
	"os/exec"
	"runtime"
	"testing"
)

// FuzzShellQuote asserts ShellQuote produces a string that `sh -c "printf %s <quoted>"`
// reproduces exactly, for arbitrary input. Skipped where no POSIX sh exists (e.g. Windows CI).
func FuzzShellQuote(f *testing.F) {
	if runtime.GOOS == "windows" {
		f.Skip("no POSIX sh on windows")
	}
	if _, err := exec.LookPath("sh"); err != nil {
		f.Skip("sh not available")
	}
	f.Add("plain")
	f.Add("with space")
	f.Add(`quote ' and $var and "dq" and ; | &`)
	f.Add("back`tick")
	f.Add("")
	f.Fuzz(func(t *testing.T, s string) {
		out, err := exec.Command("sh", "-c", "printf %s "+ShellQuote(s)).Output()
		if err != nil {
			t.Fatalf("sh rejected quoted string %q: %v", s, err)
		}
		if string(out) != s {
			t.Fatalf("round-trip mismatch: quoted %q produced %q", s, out)
		}
	})
}
