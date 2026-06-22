// Package gitlab implements forge.Forge against the GitLab Releases API.
package gitlab

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"

	"github.com/RomanAgaltsev/gantry/internal/forge"
)

// Client reads GitLab Releases for components.
type Client struct {
	baseURL string
	token   string
	marker  string
	hc      *http.Client
}

// New builds a GitLab forge client. If hc is nil, http.DefaultClient is used.
func New(baseURL, token, marker string, hc *http.Client) *Client {
	if hc == nil {
		hc = http.DefaultClient
	}
	return &Client{baseURL: baseURL, token: token, marker: marker, hc: hc}
}

type apiRelease struct {
	TagName     string `json:"tag_name"`
	Description string `json:"description"`
}

// LatestRelease returns the most recent release of the component.
func (c *Client) LatestRelease(ctx context.Context, comp forge.Component) (forge.Release, error) {
	endpoint := fmt.Sprintf("%s/api/v4/projects/%s/releases?per_page=1",
		c.baseURL, url.PathEscape(comp.Project))
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return forge.Release{}, err
	}
	req.Header.Set("PRIVATE-TOKEN", c.token)

	resp, err := c.hc.Do(req)
	if err != nil {
		return forge.Release{}, fmt.Errorf("gitlab releases for %q: %w", comp.Project, err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return forge.Release{}, fmt.Errorf("gitlab releases for %q: %s: %s", comp.Project, resp.Status, body)
	}
	var rels []apiRelease
	if err := json.NewDecoder(resp.Body).Decode(&rels); err != nil {
		return forge.Release{}, fmt.Errorf("decode releases: %w", err)
	}
	if len(rels) == 0 {
		return forge.Release{}, fmt.Errorf("component %q has no releases", comp.Project)
	}
	rel, err := forge.ParseMetadata(rels[0].Description, c.marker)
	if err != nil {
		return forge.Release{}, fmt.Errorf("component %q release %q: %w", comp.Project, rels[0].TagName, err)
	}
	return rel, nil
}
