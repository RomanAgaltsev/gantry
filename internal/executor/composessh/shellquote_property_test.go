package composessh

import (
	"os/exec"
	"runtime"
	"strings"
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
	f.Add("\x00") // NUL: cannot be represented in an exec argv (see guard below)
	f.Fuzz(func(t *testing.T, s string) {
		// A NUL byte cannot appear in a Unix argv element — execve rejects it with EINVAL —
		// so it can never reach the shell regardless of quoting. That is an OS constraint on
		// this test harness, not a ShellQuote defect; NUL is the only byte with this limit.
		if strings.ContainsRune(s, '\x00') {
			return
		}
		out, err := exec.Command("sh", "-c", "printf %s "+ShellQuote(s)).Output()
		if err != nil {
			t.Fatalf("sh rejected quoted string %q: %v", s, err)
		}
		if string(out) != s {
			t.Fatalf("round-trip mismatch: quoted %q produced %q", s, out)
		}
	})
}
