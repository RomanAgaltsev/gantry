package daemon

import (
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"io"
	"net/http"

	"github.com/RomanAgaltsev/gantry/internal/logging"
)

// tokenHeader is the shared-secret header a forge webhook must send in token mode.
const tokenHeader = "X-Gantry-Token" //nolint:gosec // a header name, not a credential

// sigHeader is the HMAC-SHA256 body-signature header (GitHub style) checked in HMAC mode.
const sigHeader = "X-Hub-Signature-256"

// bodyLimit caps the doorbell request body read for HMAC verification; a doorbell body is a
// small webhook payload, and gantry reads no version data from it (C3-D2).
const bodyLimit = 1 << 20 // 1 MiB

// NewDoorbell returns an HTTP handler that, on an authenticated POST, rings a debounced
// doorbell, plus the channel the reconcile loop reads. With hmacMode=false it checks a
// shared-secret header (constant-time); with hmacMode=true it verifies an HMAC-SHA256
// signature of the request body (GitHub's X-Hub-Signature-256) so the secret is never sent on
// the wire. The channel has capacity 1 so a burst of webhooks collapses to a single pending
// reconcile. The handler reads no version data from the request (C3-D2): it only proves the
// caller is allowed to trigger a reconcile.
func NewDoorbell(secret string, hmacMode bool) (http.Handler, <-chan struct{}) {
	bell := make(chan struct{}, 1)
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		ok := false
		if hmacMode {
			ok = authenticateHMAC(r, secret)
		} else {
			ok = authenticateToken(r, secret)
		}
		if !ok {
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

// authenticateToken accepts a request whose X-Gantry-Token or X-Gitlab-Token equals secret,
// constant-time.
func authenticateToken(r *http.Request, secret string) bool {
	for _, h := range []string{tokenHeader, "X-Gitlab-Token"} {
		got := r.Header.Get(h)
		if got != "" && subtle.ConstantTimeCompare([]byte(got), []byte(secret)) == 1 {
			return true
		}
	}
	return false
}

// authenticateHMAC verifies the request body's HMAC-SHA256 signature against secret, matching
// GitHub's "sha256=<hex>" X-Hub-Signature-256 header, constant-time.
func authenticateHMAC(r *http.Request, secret string) bool {
	sig := r.Header.Get(sigHeader)
	const prefix = "sha256="
	if len(sig) <= len(prefix) || sig[:len(prefix)] != prefix {
		return false
	}
	want, err := hex.DecodeString(sig[len(prefix):])
	if err != nil {
		return false
	}
	body, err := io.ReadAll(io.LimitReader(r.Body, bodyLimit))
	if err != nil {
		return false
	}
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	return hmac.Equal(mac.Sum(nil), want)
}
