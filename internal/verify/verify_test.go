package verify

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestComposite(t *testing.T) {
	require.NoError(t, Composite{}.Verify(context.Background())) // empty passes

	ok := stubVerifier{nil}
	bad := stubVerifier{errors.New("nope")}
	require.NoError(t, Composite{ok, ok}.Verify(context.Background()))
	err := Composite{ok, bad}.Verify(context.Background())
	require.Error(t, err)
	require.Contains(t, err.Error(), "nope")
}

func TestHTTPVerifier(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	require.NoError(t, HTTPVerifier{URL: srv.URL, Client: srv.Client()}.Verify(context.Background()))

	v := HTTPVerifier{URL: srv.URL, ExpectStatus: 503, Client: srv.Client()}
	err := v.Verify(context.Background())
	require.Error(t, err)
	require.Contains(t, err.Error(), "want 503")
}

func TestHTTPVerifier_ConnRefused(t *testing.T) {
	v := HTTPVerifier{URL: "http://127.0.0.1:0", Client: &http.Client{}}
	require.Error(t, v.Verify(context.Background()))
}

type stubVerifier struct{ err error }

func (s stubVerifier) Verify(context.Context) error { return s.err }
