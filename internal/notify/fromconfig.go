package notify

import (
	"fmt"
	"net/http"

	"github.com/RomanAgaltsev/gantry/internal/config"
)

// FromConfig builds a Dispatcher from cfg.Notifications, resolving each channel's secrets
// with res. An entry's Events become the channel's subscribed kinds (empty = all). Returns
// an empty (no-op) Dispatcher when no channels are configured. Shared by the daemon and
// the CLI notification wiring.
func FromConfig(cfg *config.Config, res config.SecretResolver) (Dispatcher, error) {
	var d Dispatcher
	for _, ch := range cfg.Notifications {
		n, err := buildNotifier(ch, res)
		if err != nil {
			return nil, err
		}
		d = append(d, Channel{Notifier: n, Events: eventSet(ch.Events)})
	}
	return d, nil
}

func buildNotifier(ch config.NotifyChannel, res config.SecretResolver) (Notifier, error) {
	switch ch.Kind {
	case "webhook":
		url, err := res.Resolve(ch.URL)
		if err != nil {
			return nil, err
		}
		chatID, err := res.Resolve(ch.ChatID)
		if err != nil {
			return nil, err
		}
		return WebhookNotifier{URL: url, ChatID: chatID, Client: &http.Client{}}, nil
	case "email":
		pw, err := res.Resolve(ch.SMTP.Password)
		if err != nil {
			return nil, err
		}
		return NewEmailNotifier(ch.SMTP.Host, ch.SMTP.Port, ch.SMTP.Username, pw, ch.From, ch.To, ch.SMTP.TLS), nil
	default:
		return nil, fmt.Errorf("notifications: unsupported kind %q", ch.Kind)
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
