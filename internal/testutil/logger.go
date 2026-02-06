package testutil

import (
	"io"
	"log/slog"
	"testing"
)

// NewTestLogger returns a logger suitable for tests.
func NewTestLogger(t *testing.T) *slog.Logger {
	t.Helper()
	return slog.New(slog.NewTextHandler(io.Discard, &slog.HandlerOptions{Level: slog.LevelDebug}))
}
