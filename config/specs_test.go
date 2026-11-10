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

func TestComponentListener_Validation(t *testing.T) {
	validListener := func(mode string) *ListenerConfig {
		return &ListenerConfig{
			Mode:                      mode,
			FederatedServicesStrategy: "auto",
		}
	}
	validKafka := func() *KafkaConfig {
		return &KafkaConfig{
			Brokers:                []string{"localhost:9092"},
			ConsumerGroup:          "authz-listener",
			TopicPartitions:        1,
			TopicReplicationFactor: 1,
		}
	}
	validExtAuthz := func() *ExtAuthzServiceConfig {
		return &ExtAuthzServiceConfig{
			JwkSetURL: "http://localhost:8080/.well-known/jwks.json",
		}
	}

	tests := []struct {
		name      string
		cfg       *Config
		wantError string
	}{
		{
			name: "platform mode valid without ExtAuthzService",
			cfg: &Config{
				Listener: validListener(ListenModePlatform),
				Kafka:    validKafka(),
			},
		},
		{
			name: "platform mode fails when brokers are empty",
			cfg: &Config{
				Listener: validListener(ListenModePlatform),
				Kafka: &KafkaConfig{
					Brokers:                nil,
					ConsumerGroup:          "authz-listener",
					TopicPartitions:        1,
					TopicReplicationFactor: 1,
				},
			},
			wantError: "kafka configuration validation failed",
		},
		{
			name: "platform mode fails when consumer_group is empty",
			cfg: &Config{
				Listener: validListener(ListenModePlatform),
				Kafka: &KafkaConfig{
					Brokers:                []string{"localhost:9092"},
					ConsumerGroup:          "",
					TopicPartitions:        1,
					TopicReplicationFactor: 1,
				},
			},
			wantError: "kafka configuration validation failed",
		},
		{
			name: "standalone mode valid with nil KafkaConfig",
			cfg: &Config{
				Listener:        validListener(ListenModeStandalone),
				Kafka:           nil,
				ExtAuthzService: validExtAuthz(),
			},
		},
		{
			name: "standalone mode fails when ExtAuthzService JwkSetURL is missing",
			cfg: &Config{
				Listener:        validListener(ListenModeStandalone),
				ExtAuthzService: &ExtAuthzServiceConfig{},
			},
			wantError: "ext_authz_service configuration validation failed",
		},
		{
			name: "standalone mode fails when strategy is config and federated_services is empty",
			cfg: &Config{
				Listener: &ListenerConfig{
					Mode:                      ListenModeStandalone,
					FederatedServicesStrategy: "config",
				},
				ExtAuthzService: validExtAuthz(),
			},
			wantError: "federated_services is required when strategy is 'config'",
		},
		{
			name: "hybrid mode valid when both Kafka and ExtAuthzService are valid",
			cfg: &Config{
				Listener:        validListener(ListenModeHybrid),
				Kafka:           validKafka(),
				ExtAuthzService: validExtAuthz(),
			},
		},
		{
			name: "hybrid mode fails when brokers are missing",
			cfg: &Config{
				Listener: validListener(ListenModeHybrid),
				Kafka: &KafkaConfig{
					ConsumerGroup:          "authz-listener",
					TopicPartitions:        1,
					TopicReplicationFactor: 1,
				},
				ExtAuthzService: validExtAuthz(),
			},
			wantError: "kafka configuration validation failed",
		},
		{
			name: "hybrid mode fails when ExtAuthzService is invalid",
			cfg: &Config{
				Listener:        validListener(ListenModeHybrid),
				Kafka:           validKafka(),
				ExtAuthzService: &ExtAuthzServiceConfig{JwkSetURL: "not-a-url"},
			},
			wantError: "ext_authz_service configuration validation failed",
		},
		{
			name: "invalid mode fails listener validation",
			cfg: &Config{
				Listener: validListener("invalid-mode"),
				Kafka:    validKafka(),
			},
			wantError: "listener configuration validation failed",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.cfg.ValidateComponents(ComponentListener)
			if tc.wantError != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tc.wantError)
			} else {
				require.NoError(t, err)
			}
		})
	}
}
