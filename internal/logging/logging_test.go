package logging

import (
	"bytes"
	"context"
	"log/slog"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestIntoFrom_RoundTrip(t *testing.T) {
	var buf bytes.Buffer
	log := New("text", "info", &buf)
	ctx := Into(context.Background(), log)

	From(ctx).Info("hello", "k", "v")

	require.Contains(t, buf.String(), "hello")
	require.Contains(t, buf.String(), "k=v")
}

func TestFrom_BareContextDiscards(t *testing.T) {
	got := From(context.Background())
	require.NotNil(t, got)
	// A discard logger is disabled at every level.
	require.False(t, got.Enabled(context.Background(), slog.LevelError))
}

func TestNew_JSONRespectsLevel(t *testing.T) {
	var buf bytes.Buffer
	log := New("json", "info", &buf)

	log.Debug("dropped-msg")
	log.Info("kept-msg")

	out := buf.String()
	require.NotContains(t, out, "dropped-msg")   // below info threshold
	require.Contains(t, out, `"msg":"kept-msg"`) // JSON format honored
	require.Contains(t, out, `"level":"INFO"`)
}
