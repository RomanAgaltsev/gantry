package notify

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/RomanAgaltsev/gantry/internal/config"
)

func TestFromConfig_BuildsChannels(t *testing.T) {
	t.Setenv("HOOK_URL", "https://hook.example/x")
	cfg := &config.Config{Notifications: []config.NotifyChannel{
		{Kind: "webhook", URL: config.SecretRef{Raw: "${env:HOOK_URL}"}, Events: []string{"deployed"}},
		{Kind: "email", SMTP: config.SMTPConfig{Host: "smtp.example", Port: 587}, From: "g@x", To: []string{"o@x"}},
	}}
	d, err := FromConfig(context.Background(), cfg, config.DefaultResolver())
	require.NoError(t, err)
	require.Len(t, d, 2)
	require.True(t, d[0].wants("deployed"))
	require.False(t, d[0].wants("promoted")) // events set restricts it
}

func TestFromConfig_EmptyIsNoop(t *testing.T) {
	d, err := FromConfig(context.Background(), &config.Config{}, config.DefaultResolver())
	require.NoError(t, err)
	require.Empty(t, d)
}

func TestFromConfig_BadSecretErrors(t *testing.T) {
	cfg := &config.Config{Notifications: []config.NotifyChannel{
		{Kind: "webhook", URL: config.SecretRef{Raw: "${env:MISSING_HOOK}"}},
	}}
	_, err := FromConfig(context.Background(), cfg, config.DefaultResolver())
	require.Error(t, err)
}
