package github

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/RomanAgaltsev/gantry/internal/forge"
)

const relBody = "<!-- gantry-release-metadata:v1:start -->\n```json\n" +
	`{"schema_version":"1","component":"svc","semver_version":"v1.2.0",` +
	`"image_repository":"reg/svc","image_tag":"v1.2.0","image_digest":"sha256:d",` +
	`"commit_sha":"c0ffee","built_at":"2026-06-18T09:00:00Z","changelog_section":"x"}` +
	"\n```\n<!-- gantry-release-metadata:v1:end -->"

func jsonString(s string) string { b, _ := json.Marshal(s); return string(b) }

func TestLatestRelease(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "Bearer tok", r.Header.Get("Authorization"))
		require.Equal(t, "/repos/octo/svc/releases/latest", r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"tag_name":"v1.2.0","body":` + jsonString(relBody) + `}`))
	}))
	defer srv.Close()

	c := New(srv.URL, "tok", "gantry-release-metadata", srv.Client())
	rel, err := c.LatestRelease(context.Background(), forge.Component{ID: "svc", Project: "octo/svc", PinKey: "SVC_IMAGE"})
	require.NoError(t, err)
	require.Equal(t, "reg/svc:v1.2.0@sha256:d", rel.ImageRef())
	require.Equal(t, "v1.2.0", rel.SemverVersion)
}

func TestLatestRelease_NotFound(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"message":"Not Found"}`))
	}))
	defer srv.Close()
	c := New(srv.URL, "tok", "gantry-release-metadata", srv.Client())
	_, err := c.LatestRelease(context.Background(), forge.Component{Project: "octo/svc"})
	require.Error(t, err)
	require.Contains(t, err.Error(), "no published")
	require.Contains(t, err.Error(), "octo/svc")
}

func TestLatestRelease_PublicNoToken(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, hasAuth := r.Header["Authorization"]
		require.False(t, hasAuth, "Authorization header must be absent when token is empty")
		_, _ = w.Write([]byte(`{"tag_name":"v1.2.0","body":` + jsonString(relBody) + `}`))
	}))
	defer srv.Close()
	c := New(srv.URL, "", "gantry-release-metadata", srv.Client())
	_, err := c.LatestRelease(context.Background(), forge.Component{Project: "octo/svc"})
	require.NoError(t, err)
}

func TestLatestRelease_BadMetadata(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"tag_name":"v1.2.0","body":"no marker here"}`))
	}))
	defer srv.Close()
	c := New(srv.URL, "tok", "gantry-release-metadata", srv.Client())
	_, err := c.LatestRelease(context.Background(), forge.Component{Project: "octo/svc"})
	require.Error(t, err)
}
