package cli

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/RomanAgaltsev/gantry/internal/config"
	"github.com/RomanAgaltsev/gantry/internal/notify"
)

func TestBuildNotifier_WebhookDispatches(t *testing.T) {
	var body []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ = io.ReadAll(r.Body)
	}))
	defer srv.Close()

	res := config.SecretResolver{
		LookupEnv: func(k string) (string, bool) {
			if k == "HOOK" {
				return srv.URL, true
			}
			return "", false
		},
	}
	d, err := buildNotifier(res, []config.NotifyChannel{
		{Kind: "webhook", URL: config.SecretRef{Raw: "${env:HOOK}"}, Events: []string{"deployed"}},
	})
	require.NoError(t, err)
	d.Dispatch(context.Background(), notify.Event{Kind: "deployed", Environment: "test", Message: "deployed 1 pin(s) to test"})
	require.Contains(t, string(body), "deployed 1 pin(s) to test")
}
