// Command forge is a fake GitLab Releases API for the gantry demo stand.
//
// It serves the single endpoint gantry calls -- GET /api/v4/projects/{project}/releases
// -- plus a control endpoint the demo driver uses to publish new releases on demand.
// State is in memory, seeded from releases.json at start, so restarting the container
// resets the stand.
package main

import (
	"encoding/json"
	"errors"
	"flag"
	"log"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
)

// metadataMarker matches metadata_marker in examples/demo/gantry.yaml. That config is
// the artifact under proof, so this constant follows it and never the other way round.
const metadataMarker = "gantry-release-metadata"

// defaultPerPage mirrors the client's releasePage when the query omits per_page.
const defaultPerPage = 20

// release is one seeded or published release, and is the hand-editable shape in
// releases.json. The GitLab payload gantry reads is rendered from it on the way out.
type release struct {
	TagName         string `json:"tag_name"`
	Component       string `json:"component"`
	SemverVersion   string `json:"semver_version"`
	ImageRepository string `json:"image_repository"`
	ImageTag        string `json:"image_tag"`
	BuiltAt         string `json:"built_at"`
}

// apiRelease is the subset of GitLab's release payload gantry actually reads
// (internal/forge/gitlab/gitlab.go): the tag and the description carrying the
// metadata block.
type apiRelease struct {
	TagName     string `json:"tag_name"`
	Description string `json:"description"`
}

// metadata is the JSON object inside the marker block. Field order here is the
// order it renders in, which keeps a hand-read release body predictable.
type metadata struct {
	SchemaVersion   string `json:"schema_version"`
	Component       string `json:"component"`
	SemverVersion   string `json:"semver_version"`
	ImageRepository string `json:"image_repository"`
	ImageTag        string `json:"image_tag"`
	BuiltAt         string `json:"built_at"`
}

// describe renders the release body gantry parses. The inner ```json fence is
// included deliberately: ParseMetadata strips it, so carrying it proves the
// stand exercises the same path a human-written GitLab release takes.
func (r release) describe() (string, error) {
	b, err := json.Marshal(metadata{
		SchemaVersion:   "1",
		Component:       r.Component,
		SemverVersion:   r.SemverVersion,
		ImageRepository: r.ImageRepository,
		ImageTag:        r.ImageTag,
		BuiltAt:         r.BuiltAt,
	})
	if err != nil {
		return "", err
	}
	var sb strings.Builder
	sb.WriteString("Release " + r.TagName + "\n\n")
	sb.WriteString("<!-- " + metadataMarker + ":v1:start -->\n")
	sb.WriteString("```json\n")
	sb.Write(b)
	sb.WriteString("\n```\n")
	sb.WriteString("<!-- " + metadataMarker + ":v1:end -->\n")
	return sb.String(), nil
}

// store holds releases per project, newest first.
type store struct {
	mu sync.RWMutex
	by map[string][]release
}

func newStore(seed map[string][]release) *store {
	if seed == nil {
		seed = map[string][]release{}
	}
	return &store{by: seed}
}

func (s *store) list(project string, limit int) []release {
	s.mu.RLock()
	defer s.mu.RUnlock()
	rels := s.by[project]
	if limit > 0 && limit < len(rels) {
		rels = rels[:limit]
	}
	out := make([]release, len(rels))
	copy(out, rels)
	return out
}

// prepend makes r the newest release of project.
func (s *store) prepend(project string, r release) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.by[project] = append([]release{r}, s.by[project]...)
}

// projectFromPath pulls the project out of /api/v4/projects/{project}/releases.
//
// The project arrives percent-encoded ("demo%2Fapi") because the client builds it
// with url.PathEscape. r.URL.Path has already decoded that %2F back into a slash,
// which would split the segment, so the escaped path is what gets parsed here.
func projectFromPath(escaped string) (string, error) {
	const prefix = "/api/v4/projects/"
	rest, ok := strings.CutPrefix(escaped, prefix)
	if !ok {
		return "", errors.New("not a projects path")
	}
	seg, ok := strings.CutSuffix(rest, "/releases")
	if !ok || seg == "" || strings.Contains(seg, "/") {
		return "", errors.New("not a releases path")
	}
	project, err := url.PathUnescape(seg)
	if err != nil {
		return "", err
	}
	return project, nil
}

func (s *store) handleReleases(w http.ResponseWriter, r *http.Request) {
	project, err := projectFromPath(r.URL.EscapedPath())
	if err != nil {
		http.NotFound(w, r)
		return
	}
	limit := defaultPerPage
	if v := r.URL.Query().Get("per_page"); v != "" {
		if n, convErr := strconv.Atoi(v); convErr == nil && n > 0 {
			limit = n
		}
	}

	rels := s.list(project, limit)
	// An unknown project is an empty array, not a 404: that is what GitLab returns
	// for a project with no releases, and gantry turns it into its own error.
	out := make([]apiRelease, 0, len(rels))
	for _, rel := range rels {
		desc, descErr := rel.describe()
		if descErr != nil {
			http.Error(w, descErr.Error(), http.StatusInternalServerError)
			return
		}
		out = append(out, apiRelease{TagName: rel.TagName, Description: desc})
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(out); err != nil {
		// The project comes from the request path, so it stays out of the log line.
		log.Printf("encode releases: %v", err)
	}
}

func loadSeed(path string) (map[string][]release, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var seed map[string][]release
	if err := json.Unmarshal(b, &seed); err != nil {
		return nil, err
	}
	return seed, nil
}

func main() {
	addr := flag.String("addr", ":443", "listen address")
	seedPath := flag.String("seed", "releases.json", "seed releases file")
	certPath := flag.String("tls-cert", "/run/secrets/forge.crt", "TLS certificate; empty serves plain HTTP")
	keyPath := flag.String("tls-key", "/run/secrets/forge.key", "TLS private key")
	flag.Parse()

	seed, err := loadSeed(*seedPath)
	if err != nil {
		log.Fatalf("load seed: %v", err)
	}
	s := newStore(seed)

	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v4/projects/", s.handleReleases)
	mux.HandleFunc("POST /_control/release", s.handleControl)

	srv := &http.Server{
		Addr:              *addr,
		Handler:           mux,
		ReadHeaderTimeout: 10 * time.Second,
	}
	// The shipped config says https://gitlab.example.com, so TLS is the default and
	// plain HTTP is the opt-out (-tls-cert=""), not the other way round. Falling back
	// to HTTP when a cert is merely missing would turn a setup failure into a
	// confusing trust error later, so a named-but-unreadable cert is fatal here.
	serve := srv.ListenAndServe
	if *certPath != "" {
		if _, statErr := os.Stat(*certPath); statErr != nil {
			log.Fatalf("tls cert %s: %v (pass -tls-cert= to serve plain HTTP)", *certPath, statErr)
		}
		serve = func() error { return srv.ListenAndServeTLS(*certPath, *keyPath) }
		log.Printf("forge listening on %s with TLS (seed %s)", *addr, *seedPath)
	} else {
		log.Printf("forge listening on %s without TLS (seed %s)", *addr, *seedPath)
	}
	if err := serve(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Fatalf("serve: %v", err)
	}
}

// controlRequest is the body of POST /_control/release. Only project, image_repository
// and image_tag are required; version defaults to a bumped-looking tag and built_at to now.
type controlRequest struct {
	Project         string `json:"project"`
	Component       string `json:"component"`
	Version         string `json:"version"`
	ImageRepository string `json:"image_repository"`
	ImageTag        string `json:"image_tag"`
	BuiltAt         string `json:"built_at"`
}

// componentOf derives the metadata component from a project path: demo/api -> api.
func componentOf(project string) string {
	if i := strings.LastIndex(project, "/"); i >= 0 {
		return project[i+1:]
	}
	return project
}

// handleControl publishes a release, making it the newest for its project.
//
// built_at is settable rather than always "now" because the drift scenario needs a
// release dated days in the past, and there is no other way to produce one.
func (s *store) handleControl(w http.ResponseWriter, r *http.Request) {
	var req controlRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "decode body: "+err.Error(), http.StatusBadRequest)
		return
	}
	switch {
	case req.Project == "":
		http.Error(w, "project is required", http.StatusBadRequest)
		return
	case req.ImageRepository == "":
		http.Error(w, "image_repository is required", http.StatusBadRequest)
		return
	case req.ImageTag == "":
		http.Error(w, "image_tag is required", http.StatusBadRequest)
		return
	}

	builtAt := req.BuiltAt
	if builtAt == "" {
		builtAt = time.Now().UTC().Format(time.RFC3339)
	} else if _, err := time.Parse(time.RFC3339, builtAt); err != nil {
		// Reject here rather than let gantry fail on it later: a bad timestamp
		// published into the stand is far harder to diagnose downstream.
		http.Error(w, "built_at must be RFC3339: "+err.Error(), http.StatusBadRequest)
		return
	}
	component := req.Component
	if component == "" {
		component = componentOf(req.Project)
	}
	version := req.Version
	if version == "" {
		version = req.ImageTag
	}

	rel := release{
		TagName:         "v" + strings.TrimPrefix(version, "v"),
		Component:       component,
		SemverVersion:   strings.TrimPrefix(version, "v"),
		ImageRepository: req.ImageRepository,
		ImageTag:        req.ImageTag,
		BuiltAt:         builtAt,
	}
	s.prepend(req.Project, rel)
	log.Printf("published %s %s -> %s:%s built %s",
		req.Project, rel.TagName, rel.ImageRepository, rel.ImageTag, rel.BuiltAt)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	if err := json.NewEncoder(w).Encode(rel); err != nil {
		log.Printf("encode control response: %v", err)
	}
}
