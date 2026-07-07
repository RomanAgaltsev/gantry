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

// PayloadShape selects the JSON body a WebhookNotifier posts: the generic structured payload,
// Telegram's sendMessage shape (chat_id + text), or Slack's minimal {text}. Named kinds are
// thin wrappers over one webhook core (review D6).
type PayloadShape string

const (
	ShapeGeneric  PayloadShape = ""         // structured fields (kind=webhook)
	ShapeTelegram PayloadShape = "telegram" // chat_id + text
	ShapeSlack    PayloadShape = "slack"    // {"text": …}
)

// WebhookNotifier POSTs an event as JSON. The payload shape is selected by Shape: the generic
// structured body (default), Telegram's sendMessage body (chat_id + text), or Slack's minimal
// {text}. Named notifier kinds (slack/telegram) are thin wrappers over this core (review D6).
type WebhookNotifier struct {
	URL    string
	ChatID string // Telegram only; ignored by the other shapes
	Shape  PayloadShape
	Client *http.Client
}

type webhookPayload struct {
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
	body, err := w.marshal(e)
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

// marshal builds the request body for the configured shape.
func (w WebhookNotifier) marshal(e Event) ([]byte, error) {
	switch w.Shape {
	case ShapeSlack:
		return json.Marshal(struct {
			Text string `json:"text"`
		}{Text: e.Message})
	case ShapeTelegram:
		return json.Marshal(struct {
			ChatID string `json:"chat_id"`
			Text   string `json:"text"`
		}{ChatID: w.ChatID, Text: e.Message})
	default:
		return json.Marshal(webhookPayload{
			Text: e.Message, Event: e.Kind, Environment: e.Environment,
			Commit: e.Commit, By: e.By, Timestamp: e.Time.UTC().Format(time.RFC3339),
		})
	}
}
