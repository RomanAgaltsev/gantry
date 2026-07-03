package daemon

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestDoorbell_ValidPostRingsOnce(t *testing.T) {
	h, bell := NewDoorbell("s3cret")

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
	h, bell := NewDoorbell("s3cret")

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
	h, bell := NewDoorbell("s3cret")

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
	h, _ := NewDoorbell("s3cret")

	rec := httptest.NewRecorder()

	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/hooks/forge", nil))

	require.Equal(t, http.StatusMethodNotAllowed, rec.Code)
}
