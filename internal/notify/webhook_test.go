package notify

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestWebhookNotify_TelegramCompatiblePayload(t *testing.T) {
	var body map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(b, &body)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	n := WebhookNotifier{URL: srv.URL, ChatID: "123"}
	err := n.Notify(context.Background(), Event{
		Kind: "deployed", Environment: "test", Commit: "1a2b3c4", By: "sync",
		Message: "deployed 3 pin(s) to test", Time: time.Unix(0, 0),
	})
	require.NoError(t, err)
	require.Equal(t, "123", body["chat_id"])                    // Telegram reads this
	require.Equal(t, "deployed 3 pin(s) to test", body["text"]) // ...and this
	require.Equal(t, "deployed", body["event"])                 // machine-readable extras
	require.Equal(t, "test", body["environment"])
}

func TestWebhookNotify_OmitsEmptyChatID(t *testing.T) {
	var body map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(b, &body)
	}))
	defer srv.Close()

	require.NoError(t, WebhookNotifier{URL: srv.URL}.Notify(context.Background(), Event{Kind: "deployed", Message: "x"}))
	_, has := body["chat_id"]
	require.False(t, has)
}

func TestWebhookNotify_Non2xxIsError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()
	require.Error(t, WebhookNotifier{URL: srv.URL}.Notify(context.Background(), Event{Kind: "deployed", Message: "x"}))
}
