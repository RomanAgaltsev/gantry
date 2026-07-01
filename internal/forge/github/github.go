package github

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/RomanAgaltsev/gantry/internal/forge"
)

// defaultTimeout bounds a single forge HTTP call so a hung GitHub can't make gantry
// hang forever (it is typically run unattended in CI).
const defaultTimeout = 30 * time.Second

// errBodyLimit caps how much of an error response body is read into the error message,
// so a misbehaving endpoint can't make gantry buffer an unbounded body.
const errBodyLimit = 4 << 10 // 4 KiB

// defaultBaseURL is the github.com API base; GitHub Enterprise sets base_url to
// https://<host>/api/v3 instead.
const defaultBaseURL = "https://api.github.com"

// Client reads GitHub Releases for components.
type Client struct {
	baseURL string
	token   string
	marker  string
	hc      *http.Client
}

// New builds a GitHub forge client. If hc is nil, a client with a sane request timeout
// (defaultTimeout) is used. An empty baseURL falls back to https://api.github.com.
func New(baseURL, token, marker string, hc *http.Client) *Client {
	if hc == nil {
		hc = &http.Client{Timeout: defaultTimeout}
	}
	if baseURL == "" {
		baseURL = defaultBaseURL
	}
	return &Client{
		baseURL: baseURL,
		token:   token,
		marker:  marker,
		hc:      hc,
	}
}

type apiRelease struct {
	TagName string `json:"tag_name"`
	Body    string `json:"body"`
}

// LatestRelease returns the latest published (non-draft, non-prerelease) release.
// GitHub's /releases/latest endpoint already excludes drafts and prereleases.
func (c *Client) LatestRelease(ctx context.Context, comp forge.Component) (forge.Release, error) {
	// comp.Project is "owner/repo"; the slash is a real path separator, so it is NOT escaped.
	endpoint := fmt.Sprintf("%s/repos/%s/releases/latest", c.baseURL, comp.Project)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return forge.Release{}, err
	}
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")

	resp, err := c.hc.Do(req)
	if err != nil {
		return forge.Release{}, fmt.Errorf("github release for %q: %w", comp.Project, err)
	}
	defer func() { _ = resp.Body.Close() }() //nolint:gosec // best-effort close of the response body
	if resp.StatusCode == http.StatusNotFound {
		return forge.Release{}, fmt.Errorf("component %q has no published (non-draft, non-prerelease) release", comp.Project)
	}
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, errBodyLimit)) //nolint:gosec // body is best-effort context for the error message
		return forge.Release{}, fmt.Errorf("github release for %q: %s: %s", comp.Project, resp.Status, body)
	}
	var rel apiRelease
	if err := json.NewDecoder(resp.Body).Decode(&rel); err != nil {
		return forge.Release{}, fmt.Errorf("decode release: %w", err)
	}
	r, err := forge.ParseMetadata(rel.Body, c.marker)
	if err != nil {
		return forge.Release{}, fmt.Errorf("component %q release %q: %w", comp.Project, rel.TagName, err)
	}
	return r, nil
}
