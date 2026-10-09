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

func TestExtAuthzServiceConfig_Validation(t *testing.T) {
	tests := []struct {
		name      string
		cfg       *ExtAuthzServiceConfig
		wantError bool
	}{
		{
			name: "valid configuration",
			cfg: &ExtAuthzServiceConfig{
				JwkSetURL:      "http://localhost:8080/.well-known/jwks.json",
				HydraJwkSetURL: "http://localhost:4444/.well-known/jwks.json",
				HydraIssuer:    "http://localhost:4444/",
			},
			wantError: false,
		},
		{
			name: "missing JwkSetURL",
			cfg: &ExtAuthzServiceConfig{
				HydraJwkSetURL: "http://localhost:4444/.well-known/jwks.json",
				HydraIssuer:    "http://localhost:4444/",
			},
			wantError: true,
		},
		{
			name: "missing HydraJwkSetURL",
			cfg: &ExtAuthzServiceConfig{
				JwkSetURL:   "http://localhost:8080/.well-known/jwks.json",
				HydraIssuer: "http://localhost:4444/",
			},
			wantError: true,
		},
		{
			name: "missing HydraIssuer",
			cfg: &ExtAuthzServiceConfig{
				JwkSetURL:      "http://localhost:8080/.well-known/jwks.json",
				HydraJwkSetURL: "http://localhost:4444/.well-known/jwks.json",
			},
			wantError: true,
		},
		{
			name: "invalid HydraJwkSetURL format",
			cfg: &ExtAuthzServiceConfig{
				JwkSetURL:      "http://localhost:8080/.well-known/jwks.json",
				HydraJwkSetURL: "not-a-valid-url",
				HydraIssuer:    "http://localhost:4444/",
			},
			wantError: true,
		},
		{
			name: "invalid HydraIssuer format",
			cfg: &ExtAuthzServiceConfig{
				JwkSetURL:      "http://localhost:8080/.well-known/jwks.json",
				HydraJwkSetURL: "http://localhost:4444/.well-known/jwks.json",
				HydraIssuer:    "ftp://invalid-scheme",
			},
			wantError: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			c := &Config{
				ExtAuthzService: tc.cfg,
			}
			err := c.ValidateComponents(ComponentExtAuthz)
			if tc.wantError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}
