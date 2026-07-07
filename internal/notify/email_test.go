package notify

import (
	"context"
	"net/smtp"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestEmailNotify_BuildsMessage(t *testing.T) {
	var gotAddr, gotFrom string
	var gotTo []string
	var gotMsg []byte
	n := NewEmailNotifier("smtp.example.com", 587, "ops", "pw", "gantry@example.com", []string{"a@x", "b@x"}, "")
	n.send = func(addr string, _ smtp.Auth, from string, to []string, msg []byte) error {
		gotAddr, gotFrom, gotTo, gotMsg = addr, from, to, msg
		return nil
	}
	err := n.Notify(context.Background(), Event{Kind: "verify_failed", Environment: "prod", Message: "verify failed for prod"})
	require.NoError(t, err)
	require.Equal(t, "smtp.example.com:587", gotAddr)
	require.Equal(t, "gantry@example.com", gotFrom)
	require.Equal(t, []string{"a@x", "b@x"}, gotTo)
	require.Contains(t, string(gotMsg), "Subject: [gantry] verify_failed prod")
	require.Contains(t, string(gotMsg), "verify failed for prod")
}

func TestEmailNotifier_StartTLSUsesSendMailSeam(t *testing.T) {
	var called bool
	n := NewEmailNotifier("mail", 587, "u", "p", "from@x", []string{"to@x"}, "starttls")
	n.send = func(string, smtp.Auth, string, []string, []byte) error { called = true; return nil }
	require.NoError(t, n.Notify(context.Background(), Event{Kind: "deployed", Environment: "prod", Message: "ok"}))
	require.True(t, called)
}

func TestEmailNotifier_ImplicitTLSTakesTLSPath(t *testing.T) {
	var dialed bool
	n := NewEmailNotifier("mail", 465, "u", "p", "from@x", []string{"to@x"}, "implicit")
	n.implicitSend = func(addr, msg string) error { dialed = true; return nil }
	require.NoError(t, n.Notify(context.Background(), Event{Kind: "deployed", Environment: "prod", Message: "ok"}))
	require.True(t, dialed)
}
