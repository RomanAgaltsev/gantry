package gitlab

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/RomanAgaltsev/gantry/internal/forge"
	"github.com/stretchr/testify/require"
)

const relBody = "<!-- gantry-release-metadata:v1:start -->\n```json\n" +
	`{"schema_version":"1","component":"svc","semver_version":"v1.2.0",` +
	`"image_repository":"reg/svc","image_tag":"v1.2.0","image_digest":"sha256:d",` +
	`"commit_sha":"c0ffee","built_at":"2026-06-18T09:00:00Z","changelog_section":"x"}` +
	"\n```\n<!-- gantry-release-metadata:v1:end -->"

func TestLatestRelease(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "tok", r.Header.Get("PRIVATE-TOKEN"))
		require.Equal(t, "/api/v4/projects/grp/svc/releases", r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[{"tag_name":"v1.2.0","description":` + jsonString(relBody) + `}]`))
	}))
	defer srv.Close()

	c := New(srv.URL, "tok", "gantry-release-metadata", srv.Client())
	rel, err := c.LatestRelease(context.Background(), forge.Component{ID: "svc", Project: "grp/svc", PinKey: "SVC_IMAGE"})
	require.NoError(t, err)
	require.Equal(t, "reg/svc:v1.2.0@sha256:d", rel.ImageRef())
}

func TestLatestRelease_NoReleases(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`[]`))
	}))
	defer srv.Close()
	c := New(srv.URL, "tok", "gantry-release-metadata", srv.Client())
	_, err := c.LatestRelease(context.Background(), forge.Component{Project: "grp/svc"})
	require.Error(t, err)
}

func jsonString(s string) string {
	b, _ := json.Marshal(s)
	return string(b)
}
