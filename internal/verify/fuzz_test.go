package verify

import "testing"

// FuzzParseComposePS asserts the compose-ps output parser never panics on arbitrary input,
// tolerating both the JSON-array and newline-delimited JSON-object shapes. Errors are expected;
// only a panic would be a bug.
func FuzzParseComposePS(f *testing.F) {
	f.Add(`[{"Service":"web","State":"running"}]`)
	f.Add(`{"Name":"web","State":"exited"}` + "\n" + `{"Name":"db","State":"running"}`)
	f.Add(`garbage`)
	f.Add("")
	f.Fuzz(func(t *testing.T, out string) {
		_, _ = parseComposePS(out)
	})
}
