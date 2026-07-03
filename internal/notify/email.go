package notify

import (
	"context"
	"fmt"
	"net/smtp"
	"strings"
)

// sendFunc matches smtp.SendMail so tests can inject a fake transport.
type sendFunc func(addr string, a smtp.Auth, from string, to []string, msg []byte) error

// EmailNotifier sends one email per event over SMTP.
type EmailNotifier struct {
	Host     string
	Port     int
	Username string
	Password string
	From     string
	To       []string
	send     sendFunc // defaults to smtp.SendMail; overridable in tests
}

// NewEmailNotifier builds an EmailNotifier using the real smtp.SendMail transport.
func NewEmailNotifier(host string, port int, username, password, from string, to []string) EmailNotifier {
	return EmailNotifier{
		Host: host, Port: port, Username: username, Password: password,
		From: from, To: to, send: smtp.SendMail,
	}
}

// Notify sends the event as a single-line email. ctx is accepted for interface symmetry;
// net/smtp offers no context hook, and the Dispatcher already bounds each send with a timeout.
func (n EmailNotifier) Notify(_ context.Context, e Event) error {
	send := n.send
	if send == nil {
		send = smtp.SendMail
	}
	subject := fmt.Sprintf("[gantry] %s %s", e.Kind, e.Environment)
	msg := fmt.Sprintf("From: %s\r\nTo: %s\r\nSubject: %s\r\n\r\n%s\r\n",
		n.From, strings.Join(n.To, ", "), subject, e.Message)
	var auth smtp.Auth
	if n.Username != "" {
		auth = smtp.PlainAuth("", n.Username, n.Password, n.Host)
	}
	addr := fmt.Sprintf("%s:%d", n.Host, n.Port)
	if err := send(addr, auth, n.From, n.To, []byte(msg)); err != nil {
		return fmt.Errorf("send email: %w", err)
	}
	return nil
}
