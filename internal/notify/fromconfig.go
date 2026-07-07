package notify

import (
	"context"
	"fmt"
	"net/http"

	"github.com/RomanAgaltsev/gantry/internal/config"
)

// FromConfig builds a Dispatcher from cfg.Notifications, resolving each channel's secrets
// with res under ctx. An entry's Events become the channel's subscribed kinds (empty = all).
// Returns an empty (no-op) Dispatcher when no channels are configured. Shared by the daemon
// and the CLI notification wiring.
func FromConfig(ctx context.Context, cfg *config.Config, res config.SecretResolver) (Dispatcher, error) {
	var d Dispatcher
	for _, ch := range cfg.Notifications {
		n, err := buildNotifier(ctx, ch, res)
		if err != nil {
			return nil, err
		}
		d = append(d, Channel{Notifier: n, Events: eventSet(ch.Events)})
	}
	return d, nil
}

func buildNotifier(ctx context.Context, ch config.NotifyChannel, res config.SecretResolver) (Notifier, error) {
	switch ch.Kind {
	case "webhook", "slack", "telegram":
		url, err := res.Resolve(ctx, ch.URL)
		if err != nil {
			return nil, err
		}
		chatID, err := res.Resolve(ctx, ch.ChatID)
		if err != nil {
			return nil, err
		}
		return WebhookNotifier{URL: url, ChatID: chatID, Shape: PayloadShape(shapeFor(ch.Kind)), Client: &http.Client{}}, nil
	case "email":
		pw, err := res.Resolve(ctx, ch.SMTP.Password)
		if err != nil {
			return nil, err
		}
		return NewEmailNotifier(ch.SMTP.Host, ch.SMTP.Port, ch.SMTP.Username, pw, ch.From, ch.To, ch.SMTP.TLS), nil
	default:
		return nil, fmt.Errorf("notifications: unsupported kind %q", ch.Kind)
	}
}

// shapeFor maps a config notification kind to a webhook payload shape.
func shapeFor(kind string) string {
	switch kind {
	case "slack":
		return "slack"
	case "telegram":
		return "telegram"
	default:
		return "" // generic webhook
	}
}

func eventSet(events []string) map[string]bool {
	if len(events) == 0 {
		return nil
	}
	m := make(map[string]bool, len(events))
	for _, e := range events {
		m[e] = true
	}
	return m
}
