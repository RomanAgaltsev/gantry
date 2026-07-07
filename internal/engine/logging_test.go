package engine

import (
	"bytes"
	"context"
	"log/slog"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/RomanAgaltsev/gantry/internal/config"
	"github.com/RomanAgaltsev/gantry/internal/forge"
	"github.com/RomanAgaltsev/gantry/internal/logging"
	"github.com/RomanAgaltsev/gantry/internal/pin"
)

func TestSync_EmitsStructuredLogs(t *testing.T) {
	var buf bytes.Buffer
	ctx := logging.Into(context.Background(), slog.New(slog.NewJSONHandler(&buf, nil)))

	cfg := &config.Config{
		Components: []config.Component{
			{ID: "svc", Project: "g/svc", PinKey: "SVC_IMAGE", Source: config.ComponentSource{Forge: "release"}},
		},
		Environments: []config.Environment{
			{Name: "test", Source: config.Source{Track: "latest"}, PinFile: ".env.versions.test"},
		},
	}
	f := fakeForge{rel: forge.Release{ImageRepository: "reg/svc", ImageTag: "v1"}}
	store := &fakeStore{cur: pin.Set{}} // empty → diff writes SVC_IMAGE
	led := &fakeLedger{}
	ex := &fakeExec{}

	_, err := (&Engine{Cfg: cfg, Forge: f, Store: store, Ledger: led}).Sync(ctx, "test", ex, nil, SyncOptions{})
	require.NoError(t, err)

	out := buf.String()
	require.Contains(t, out, "polling forge")
	require.Contains(t, out, "pin written")
}
