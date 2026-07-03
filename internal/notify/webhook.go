package notify

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

const defaultWebhookTimeout = 10 * time.Second

// WebhookNotifier POSTs an event as JSON. The payload is Telegram-compatible: a Telegram Bot
// API sendMessage endpoint reads chat_id and text and ignores the extra fields, while a
// generic consumer can read the structured fields.
type WebhookNotifier struct {
	URL    string
	ChatID string // optional; included only when non-empty (Telegram)
	Client *http.Client
}

type webhookPayload struct {
	ChatID      string `json:"chat_id,omitempty"`
	Text        string `json:"text"`
	Event       string `json:"event"`
	Environment string `json:"environment"`
	Commit      string `json:"commit,omitempty"`
	By          string `json:"by,omitempty"`
	Timestamp   string `json:"timestamp"`
}

// Notify POSTs the event payload to the webhook URL. A non-2xx response is an error.
func (w WebhookNotifier) Notify(ctx context.Context, e Event) error {
	client := w.Client
	if client == nil {
		client = &http.Client{Timeout: defaultWebhookTimeout}
	}
	body, err := json.Marshal(webhookPayload{
		ChatID:      w.ChatID,
		Text:        e.Message,
		Event:       e.Kind,
		Environment: e.Environment,
		Commit:      e.Commit,
		By:          e.By,
		Timestamp:   e.Time.UTC().Format(time.RFC3339),
	})
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, w.URL, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("post webhook: %w", err)
	}
	defer func() { _ = resp.Body.Close() }() //nolint:gosec // best-effort close
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("webhook returned %s", resp.Status)
	}
	return nil
}
