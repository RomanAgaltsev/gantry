package cli

import (
	"github.com/RomanAgaltsev/gantry/internal/config"
	"github.com/RomanAgaltsev/gantry/internal/notify"
)

// buildNotifier turns the config notifications block into a Dispatcher, resolving each
// channel's secrets (webhook url/chat_id, smtp password).
func buildNotifier(res config.SecretResolver, channels []config.NotifyChannel) (notify.Dispatcher, error) {
	var d notify.Dispatcher
	for _, ch := range channels {
		events := eventSet(ch.Events)
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
			d = append(d, notify.Channel{Notifier: notify.WebhookNotifier{URL: url, ChatID: chatID}, Events: events})
		case "email":
			pw, err := res.Resolve(ch.SMTP.Password)
			if err != nil {
				return nil, err
			}
			d = append(d, notify.Channel{
				Notifier: notify.NewEmailNotifier(ch.SMTP.Host, ch.SMTP.Port, ch.SMTP.Username, pw, ch.From, ch.To),
				Events:   events,
			})
		}
	}
	return d, nil
}

// eventSet builds a subscription set; nil (all kinds) when no events are listed.
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
