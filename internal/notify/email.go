package notify

import (
	"context"
	"crypto/tls"
	"fmt"
	"net/smtp"
	"strings"
)

// sendFunc matches smtp.SendMail so tests can inject a fake transport.
type sendFunc func(addr string, a smtp.Auth, from string, to []string, msg []byte) error

// EmailNotifier sends one email per event over SMTP. tls selects the transport: "" or
// "starttls" uses smtp.SendMail (opportunistic STARTTLS); "implicit" dials TLS first (465).
type EmailNotifier struct {
	Host         string
	Port         int
	Username     string
	Password     string
	From         string
	To           []string
	tls          string
	send         sendFunc                     // starttls transport; defaults to smtp.SendMail
	implicitSend func(addr, msg string) error // implicit-TLS transport; defaults to (EmailNotifier).sendImplicit
}

// NewEmailNotifier builds an EmailNotifier. tls is "" / "starttls" (opportunistic STARTTLS via
// smtp.SendMail) or "implicit" (TLS-on-connect, port 465).
func NewEmailNotifier(host string, port int, username, password, from string, to []string, tls string) EmailNotifier {
	return EmailNotifier{
		Host: host, Port: port, Username: username, Password: password,
		From: from, To: to, tls: tls, send: smtp.SendMail,
	}
}

// Notify sends the event as a single-line email over the configured transport. ctx is accepted
// for interface symmetry; net/smtp offers no context hook, and the Dispatcher already bounds
// each send with a timeout.
func (n EmailNotifier) Notify(_ context.Context, e Event) error {
	subject := fmt.Sprintf("[gantry] %s %s", e.Kind, e.Environment)
	msg := fmt.Sprintf("From: %s\r\nTo: %s\r\nSubject: %s\r\n\r\n%s\r\n",
		n.From, strings.Join(n.To, ", "), subject, e.Message)
	addr := fmt.Sprintf("%s:%d", n.Host, n.Port)

	if n.tls == "implicit" {
		send := n.implicitSend
		if send == nil {
			send = n.sendImplicit
		}
		if err := send(addr, msg); err != nil {
			return fmt.Errorf("send email (implicit tls): %w", err)
		}
		return nil
	}

	send := n.send
	if send == nil {
		send = smtp.SendMail
	}
	var auth smtp.Auth
	if n.Username != "" {
		auth = smtp.PlainAuth("", n.Username, n.Password, n.Host)
	}
	if err := send(addr, auth, n.From, n.To, []byte(msg)); err != nil {
		return fmt.Errorf("send email: %w", err)
	}
	return nil
}

// sendImplicit delivers msg over an implicit-TLS (port 465) SMTP connection: a raw TLS dial
// followed by an smtp client handshake, auth, and DATA. Split out from Notify so the implicit
// path can be stubbed in tests.
func (n EmailNotifier) sendImplicit(addr, msg string) error {
	// The implicit-TLS path has no net/smtp context hook; the Dispatcher already bounds the
	// whole send with a timeout, so a background dial here is acceptable (noctx).
	d := tls.Dialer{Config: &tls.Config{ServerName: n.Host, MinVersion: tls.VersionTLS12}}
	conn, err := d.DialContext(context.Background(), "tcp", addr)
	if err != nil {
		return err
	}
	c, err := smtp.NewClient(conn, n.Host)
	if err != nil {
		return err
	}
	defer func() { _ = c.Quit() }() //nolint:gosec // best-effort quit
	if n.Username != "" {
		if err := c.Auth(smtp.PlainAuth("", n.Username, n.Password, n.Host)); err != nil {
			return err
		}
	}
	if err := c.Mail(n.From); err != nil {
		return err
	}
	for _, rcpt := range n.To {
		if err := c.Rcpt(rcpt); err != nil {
			return err
		}
	}
	w, err := c.Data()
	if err != nil {
		return err
	}
	if _, err := w.Write([]byte(msg)); err != nil {
		return err
	}
	return w.Close()
}
