// Package logging provides gantry's slog logger and a context carrier so the
// engine can emit structured diagnostics without taking a logger parameter.
package logging

import (
	"context"
	"io"
	"log/slog"
	"strings"
)

// New builds a slog.Logger writing to w. format is "text" or "json" (anything
// else falls back to text); level is "debug"|"info"|"warn"|"error" (anything
// else falls back to info).
func New(format, level string, w io.Writer) *slog.Logger {
	var lvl slog.Level
	switch strings.ToLower(level) {
	case "debug":
		lvl = slog.LevelDebug
	case "warn":
		lvl = slog.LevelWarn
	case "error":
		lvl = slog.LevelError
	default:
		lvl = slog.LevelInfo
	}
	opts := &slog.HandlerOptions{Level: lvl}

	var h slog.Handler
	if strings.ToLower(format) == "json" {
		h = slog.NewJSONHandler(w, opts)
	} else {
		h = slog.NewTextHandler(w, opts)
	}
	return slog.New(h)
}

type ctxKey struct{}

// Into returns a copy of ctx carrying log.
func Into(ctx context.Context, log *slog.Logger) context.Context {
	return context.WithValue(ctx, ctxKey{}, log)
}

// From returns the logger carried by ctx, or a logger that discards every
// record when none is present, so unwired callers and tests stay silent.
func From(ctx context.Context) *slog.Logger {
	if log, ok := ctx.Value(ctxKey{}).(*slog.Logger); ok && log != nil {
		return log
	}
	return slog.New(slog.DiscardHandler)
}
