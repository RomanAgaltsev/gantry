package forge

import "testing"

// FuzzParseMetadata asserts the release-body metadata extractor never panics on arbitrary input.
// Errors are expected; only a panic would be a bug.
func FuzzParseMetadata(f *testing.F) {
	marker := "gantry-release-metadata"
	f.Add("<!-- gantry-release-metadata:v1:start -->{}<!-- gantry-release-metadata:v1:end -->")
	f.Add("<!-- gantry-release-metadata:v1:start -->" + `{"image_repository":"reg/svc","image_tag":"v1","built_at":"2024-01-01T00:00:00Z"}` + "<!-- gantry-release-metadata:v1:end -->")
	f.Add("no metadata here")
	f.Add("<!-- gantry-release-metadata:v1:start --><!-- gantry-release-metadata:v1:end -->")
	f.Fuzz(func(t *testing.T, body string) {
		_, _ = ParseMetadata(body, marker)
	})
}
