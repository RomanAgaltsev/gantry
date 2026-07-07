// Package gitlab implements forge.Forge against the GitLab Releases API.
package gitlab

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"

	"github.com/RomanAgaltsev/gantry/internal/forge"
)

// defaultTimeout bounds a single forge HTTP call so a hung/black-holed GitLab
// can't make gantry hang forever (it is typically run unattended in CI).
const defaultTimeout = 30 * time.Second

// errBodyLimit caps how much of an error response body is read into the error message,
// so a misbehaving endpoint can't make gantry buffer an unbounded body.
const errBodyLimit = 4 << 10 // 4 KiB

// Client reads GitLab Releases for components.
type Client struct {
	baseURL string
	token   string
	marker  string
	hc      *http.Client
}

// New builds a GitLab forge client. If hc is nil, a client with a sane request
// timeout (defaultTimeout) is used so calls can't hang indefinitely.
func New(baseURL, token, marker string, hc *http.Client) *Client {
	if hc == nil {
		hc = &http.Client{Timeout: defaultTimeout}
	}
	return &Client{baseURL: baseURL, token: token, marker: marker, hc: hc}
}

type apiRelease struct {
	TagName     string `json:"tag_name"`
	Description string `json:"description"`
}

// releasePage bounds how many recent releases we scan for the newest stable one; enough to
// step over a run of prereleases without an unbounded fetch.
const releasePage = 20

// LatestRelease returns the most recent non-prerelease release of the component, aligning
// GitLab (which would otherwise return an RC) with GitHub's /releases/latest (D5).
func (c *Client) LatestRelease(ctx context.Context, comp forge.Component) (forge.Release, error) {
	endpoint := fmt.Sprintf("%s/api/v4/projects/%s/releases?per_page=%d",
		c.baseURL, url.PathEscape(comp.Project), releasePage)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return forge.Release{}, err
	}
	req.Header.Set("PRIVATE-TOKEN", c.token)

	resp, err := c.hc.Do(req)
	if err != nil {
		return forge.Release{}, fmt.Errorf("gitlab releases for %q: %w", comp.Project, err)
	}
	defer func() { _ = resp.Body.Close() }() //nolint:gosec // best-effort close of the response body
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, errBodyLimit)) //nolint:gosec // body is best-effort context for the error message
		return forge.Release{}, fmt.Errorf("gitlab releases for %q: %s: %s", comp.Project, resp.Status, body)
	}
	var rels []apiRelease
	if err := json.NewDecoder(resp.Body).Decode(&rels); err != nil {
		return forge.Release{}, fmt.Errorf("decode releases: %w", err)
	}
	if len(rels) == 0 {
		return forge.Release{}, fmt.Errorf("component %q has no releases", comp.Project)
	}
	var firstErr error
	for _, ar := range rels {
		rel, err := forge.ParseMetadata(ar.Description, c.marker)
		if err != nil {
			if firstErr == nil {
				firstErr = fmt.Errorf("component %q release %q: %w", comp.Project, ar.TagName, err)
			}
			continue // a release without a valid metadata block is not a gantry release; skip it
		}
		if forge.IsPrerelease(rel.SemverVersion) {
			continue // RC/beta: excluded, matching GitHub's /releases/latest (D5)
		}
		return rel, nil
	}
	if firstErr != nil {
		return forge.Release{}, firstErr
	}
	return forge.Release{}, fmt.Errorf("component %q has no non-prerelease release in the latest %d", comp.Project, releasePage)
}
