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
				JwkSetURL: "http://localhost:8080/.well-known/jwks.json",
			},
			wantError: false,
		},
		{
			name:      "missing JwkSetURL",
			cfg:       &ExtAuthzServiceConfig{},
			wantError: true,
		},
		{
			name: "invalid JwkSetURL format",
			cfg: &ExtAuthzServiceConfig{
				JwkSetURL: "not-a-valid-url",
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

func TestPostgresConfig_DSN(t *testing.T) {
	p := &PostgresConfig{
		Host:           "db.local",
		Port:           5432,
		User:           "authz",
		Password:       "secret",
		DBName:         "authz_db",
		SSLMode:        "disable",
		ConnectTimeout: 10000000000, // 10s
	}
	assert.Equal(t, "host=db.local port=5432 user=authz password=secret dbname=authz_db sslmode=disable connect_timeout=10", p.DSN())
}

func TestSTSConfig_CreateSTSConnection(t *testing.T) {
	s := &STSConfig{
		Address:              "localhost:9090",
		UseTLS:               false,
		EagerConnectionCheck: false,
	}
	conn, err := s.CreateSTSConnection()
	require.NoError(t, err)
	require.NotNil(t, conn)
	_ = conn.Close()
}

func TestServerAndMetricsAddresses(t *testing.T) {
	srv := &ServerConfig{Host: "127.0.0.1", GRPCPort: 9091, HTTPPort: 8070}
	assert.Equal(t, "127.0.0.1:9091", srv.GetGRPCAddress())
	assert.Equal(t, "127.0.0.1:8070", srv.GetHTTPAddress())

	m := &MetricsConfig{Port: 9100}
	assert.Equal(t, "127.0.0.1:9100", m.GetAddress("127.0.0.1"))
}

func TestParseLogLevel(t *testing.T) {
	assert.Equal(t, slog.LevelDebug, ParseLogLevel("debug"))
	assert.Equal(t, slog.LevelInfo, ParseLogLevel("info"))
	assert.Equal(t, slog.LevelWarn, ParseLogLevel("warn"))
	assert.Equal(t, slog.LevelError, ParseLogLevel("error"))
	assert.Equal(t, slog.LevelInfo, ParseLogLevel("unknown"))
}
