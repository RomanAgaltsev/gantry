package verify

import (
	"context"
	"fmt"
	"net/http"
	"time"
)

const defaultHTTPTimeout = 10 * time.Second

// HTTPVerifier issues a GET to URL and asserts the response status (gantry-side).
type HTTPVerifier struct {
	URL          string
	ExpectStatus int          // default 200
	Client       *http.Client // default: a client with a 10s timeout
}

// Verify performs the GET and checks the status code.
func (v HTTPVerifier) Verify(ctx context.Context) error {
	client := v.Client
	if client == nil {
		client = &http.Client{Timeout: defaultHTTPTimeout}
	}
	want := v.ExpectStatus
	if want == 0 {
		want = http.StatusOK
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, v.URL, nil)
	if err != nil {
		return err
	}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("GET %s: %w", v.URL, err)
	}
	defer func() { _ = resp.Body.Close() }() //nolint:gosec // best-effort close
	if resp.StatusCode != want {
		return fmt.Errorf("GET %s: status %d, want %d", v.URL, resp.StatusCode, want)
	}
	return nil
}
