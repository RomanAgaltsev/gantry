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

	n := WebhookNotifier{URL: srv.URL, ChatID: "123", Shape: ShapeTelegram}
	err := n.Notify(context.Background(), Event{
		Kind: "deployed", Environment: "test", Commit: "1a2b3c4", By: "sync",
		Message: "deployed 3 pin(s) to test", Time: time.Unix(0, 0),
	})
	require.NoError(t, err)
	require.Equal(t, "123", body["chat_id"])                    // Telegram reads this
	require.Equal(t, "deployed 3 pin(s) to test", body["text"]) // ...and this
	require.NotContains(t, body, "event")                       // Telegram shape carries no structured extras
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

func TestWebhook_PayloadShapes(t *testing.T) {
	cases := map[PayloadShape]func(t *testing.T, body string){
		ShapeSlack: func(t *testing.T, b string) { require.NotContains(t, b, "chat_id"); require.Contains(t, b, `"text"`) },
		ShapeTelegram: func(t *testing.T, b string) {
			require.Contains(t, b, `"chat_id":"123"`)
			require.Contains(t, b, `"text"`)
		},
		ShapeGeneric: func(t *testing.T, b string) {
			require.Contains(t, b, `"event"`)
			require.Contains(t, b, `"environment"`)
		},
	}
	for shape, check := range cases {
		var got string
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			b, _ := io.ReadAll(r.Body)
			got = string(b)
			w.WriteHeader(http.StatusOK)
		}))
		n := WebhookNotifier{URL: srv.URL, ChatID: "123", Shape: shape, Client: srv.Client()}
		require.NoError(t, n.Notify(context.Background(), Event{Kind: "deployed", Environment: "prod", Message: "ok"}))
		check(t, got)
		srv.Close()
	}
}
