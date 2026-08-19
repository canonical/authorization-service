// Copyright 2026 Canonical Ltd.
// SPDX-License-Identifier: AGPL-3.0-only

package config

import (
	"encoding/json"
	"io"
	"log/slog"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// captureStdout redirects os.Stdout for the duration of fn and returns what
// was written to it. SetupLogger writes directly to os.Stdout, so this is
// the only way to observe its output without changing its signature.
func captureStdout(t *testing.T, fn func()) []byte {
	t.Helper()

	r, w, err := os.Pipe()
	require.NoError(t, err)

	original := os.Stdout
	os.Stdout = w
	defer func() { os.Stdout = original }()

	fn()

	require.NoError(t, w.Close())
	out, err := io.ReadAll(r)
	require.NoError(t, err)
	return out
}

func TestLoggingConfig_SetupLogger_JSON(t *testing.T) {
	cfg := &LoggingConfig{Level: "info", Format: "json"}

	out := captureStdout(t, func() {
		logger := cfg.SetupLogger("authorization-service", "v1.2.3")
		logger.Info("hello")
	})

	var entry map[string]any
	require.NoError(t, json.Unmarshal(out, &entry))
	assert.Equal(t, "authorization-service", entry["service"])
	assert.Equal(t, "v1.2.3", entry["version"])
	assert.Equal(t, "hello", entry["msg"])
}

func TestLoggingConfig_SetupLogger_RespectsLevel(t *testing.T) {
	cfg := &LoggingConfig{Level: "warn", Format: "json"}

	out := captureStdout(t, func() {
		logger := cfg.SetupLogger("authorization-service", "v1.2.3")
		logger.Info("should be filtered out")
	})

	assert.Empty(t, out)
}

func TestLoggingConfig_SetupLogger_AddSource(t *testing.T) {
	cfg := &LoggingConfig{Level: "info", Format: "json", AddSource: true}

	out := captureStdout(t, func() {
		logger := cfg.SetupLogger("authorization-service", "v1.2.3")
		logger.Info("hello")
	})

	var entry map[string]any
	require.NoError(t, json.Unmarshal(out, &entry))
	assert.Contains(t, entry, slog.SourceKey)
}

func TestLoggingConfig_SetupLogger_TextFormat(t *testing.T) {
	cfg := &LoggingConfig{Level: "info", Format: "text"}

	out := captureStdout(t, func() {
		logger := cfg.SetupLogger("authorization-service", "v1.2.3")
		logger.Info("hello")
	})

	assert.Contains(t, string(out), "service=authorization-service")
	assert.Contains(t, string(out), "version=v1.2.3")
}
