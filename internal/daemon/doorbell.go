package daemon

import (
	"crypto/subtle"
	"net/http"

	"github.com/RomanAgaltsev/gantry/internal/logging"
)

// tokenHeader is the shared-secret header a forge webhook must send.
// (For forges that sign the body — GitHub's X-Hub-Signature-256, GitLab's
// X-Gitlab-Token — authenticate accepts a matching token header too; see authenticate.)
const tokenHeader = "X-Gantry-Token" //nolint:gosec // a header name, not a credential

// NewDoorbell returns an HTTP handler that, on an authenticated POST, rings a debounced
// doorbell, and the channel the reconcile loop reads. The channel has capacity 1 so a burst
// of webhooks collapses to a single pending reconcile. The handler reads no version data
// from the request (C3-D2): it only proves the caller is allowed to trigger a reconcile.
func NewDoorbell(secret string) (http.Handler, <-chan struct{}) {
	bell := make(chan struct{}, 1)
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		if !authenticate(r, secret) {
			logging.From(r.Context()).Warn("doorbell rejected an unauthenticated request", "remote", r.RemoteAddr)
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		select {
		case bell <- struct{}{}: // ring
		default: // already pending: debounce
		}
		w.WriteHeader(http.StatusAccepted)
	})
	return h, bell
}

// authenticate accepts a request whose X-Gantry-Token (or X-Gitlab-Token) equals secret,
// using a constant-time compare. (GitHub HMAC support can be added here later behind the
// same seam.)
func authenticate(r *http.Request, secret string) bool {
	for _, h := range []string{tokenHeader, "X-Gitlab-Token"} {
		got := r.Header.Get(h)
		if got != "" && subtle.ConstantTimeCompare([]byte(got), []byte(secret)) == 1 {
			return true
		}
	}
	return false
}
