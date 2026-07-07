package daemon

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestDoorbell_ValidPostRingsOnce(t *testing.T) {
	h, bell := NewDoorbell("s3cret", false)

	rec := httptest.NewRecorder()

	req := httptest.NewRequest(http.MethodPost, "/hooks/forge", nil)

	req.Header.Set("X-Gantry-Token", "s3cret")

	h.ServeHTTP(rec, req)

	require.Equal(t, http.StatusAccepted, rec.Code)

	select {

	case <-bell:

	default:

		t.Fatal("valid POST did not ring the doorbell")

	}
}

func TestDoorbell_WrongSecretRejected(t *testing.T) {
	h, bell := NewDoorbell("s3cret", false)

	rec := httptest.NewRecorder()

	req := httptest.NewRequest(http.MethodPost, "/hooks/forge", nil)

	req.Header.Set("X-Gantry-Token", "wrong")

	h.ServeHTTP(rec, req)

	require.Equal(t, http.StatusUnauthorized, rec.Code)

	select {

	case <-bell:

		t.Fatal("unauthorized POST must not ring the doorbell")

	default:

	}
}

func TestDoorbell_BurstDebouncesToOne(t *testing.T) {
	h, bell := NewDoorbell("s3cret", false)

	for range 3 {

		rec := httptest.NewRecorder()

		req := httptest.NewRequest(http.MethodPost, "/hooks/forge", nil)

		req.Header.Set("X-Gantry-Token", "s3cret")

		h.ServeHTTP(rec, req)

		require.Equal(t, http.StatusAccepted, rec.Code) // every ring is Accepted...

	}

	// ...but only one reconcile is pending.

	<-bell

	select {

	case <-bell:

		t.Fatal("expected exactly one pending ring after a burst")

	default:

	}
}

func TestDoorbell_GetRejected(t *testing.T) {
	h, _ := NewDoorbell("s3cret", false)

	rec := httptest.NewRecorder()

	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/hooks/forge", nil))

	require.Equal(t, http.StatusMethodNotAllowed, rec.Code)
}

func TestDoorbell_HMACSignature(t *testing.T) {
	secret := "s3cr3t"
	h, bell := NewDoorbell(secret, true)
	body := []byte(`{"ref":"refs/tags/v1.2.3"}`)

	sign := func(key, b []byte) string {
		m := hmac.New(sha256.New, key)
		m.Write(b)
		return "sha256=" + hex.EncodeToString(m.Sum(nil))
	}

	// Valid signature ⇒ 202 and the bell rings.
	req := httptest.NewRequest(http.MethodPost, "/hooks/forge", bytes.NewReader(body))
	req.Header.Set("X-Hub-Signature-256", sign([]byte(secret), body))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	require.Equal(t, http.StatusAccepted, rec.Code)
	select {
	case <-bell:
	default:
		t.Fatal("valid HMAC did not ring the doorbell")
	}

	// Tampered body ⇒ 401.
	req = httptest.NewRequest(http.MethodPost, "/hooks/forge", bytes.NewReader([]byte(`{"ref":"evil"}`)))
	req.Header.Set("X-Hub-Signature-256", sign([]byte(secret), body)) // signature of the *original* body
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	require.Equal(t, http.StatusUnauthorized, rec.Code)

	// Wrong secret ⇒ 401.
	req = httptest.NewRequest(http.MethodPost, "/hooks/forge", bytes.NewReader(body))
	req.Header.Set("X-Hub-Signature-256", sign([]byte("wrong"), body))
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	require.Equal(t, http.StatusUnauthorized, rec.Code)
}
