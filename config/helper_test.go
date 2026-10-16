// Copyright 2026 Canonical Ltd.
// SPDX-License-Identifier: AGPL-3.0-only

package config

import (
	"log/slog"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/trace/noop"

	"github.com/canonical/authorization-service/internal/metrics"
)

func TestBuildServiceRegistry_Strategies(t *testing.T) {
	t.Run("nil config returns empty registry", func(t *testing.T) {
		reg, err := BuildServiceRegistry(nil)
		require.NoError(t, err)
		assert.Empty(t, reg.Slugs())
	})

	t.Run("strategy fs uses ModelFS scanning excluding dummy and core", func(t *testing.T) {
		cfg := &KafkaConfig{
			FederatedServicesStrategy: "fs",
			FederatedServices:         []string{"custom-ignored"},
		}
		reg, err := BuildServiceRegistry(cfg)
		require.NoError(t, err)
		assert.Contains(t, reg.Slugs(), "tenant-service")
		assert.NotContains(t, reg.Slugs(), "dummy")
		assert.NotContains(t, reg.Slugs(), "core")
		assert.NotContains(t, reg.Slugs(), "custom-ignored")
	})

	t.Run("strategy config uses explicit slice", func(t *testing.T) {
		cfg := &KafkaConfig{
			FederatedServicesStrategy: "config",
			FederatedServices:         []string{"payments", "invoicing"},
		}
		reg, err := BuildServiceRegistry(cfg)
		require.NoError(t, err)
		assert.Equal(t, []string{"payments", "invoicing"}, reg.Slugs())
	})

	t.Run("strategy auto with empty slice falls back to FS scanning", func(t *testing.T) {
		cfg := &KafkaConfig{
			FederatedServicesStrategy: "",
			FederatedServices:         nil,
		}
		reg, err := BuildServiceRegistry(cfg)
		require.NoError(t, err)
		assert.Contains(t, reg.Slugs(), "tenant-service")
		assert.NotContains(t, reg.Slugs(), "dummy")
		assert.NotContains(t, reg.Slugs(), "core")
	})

	t.Run("strategy auto with non-empty slice uses config slice", func(t *testing.T) {
		cfg := &KafkaConfig{
			FederatedServicesStrategy: "auto",
			FederatedServices:         []string{"billing"},
		}
		reg, err := BuildServiceRegistry(cfg)
		require.NoError(t, err)
		assert.Equal(t, []string{"billing"}, reg.Slugs())
	})

	t.Run("unknown strategy returns error", func(t *testing.T) {
		cfg := &KafkaConfig{
			FederatedServicesStrategy: "invalid",
		}
		_, err := BuildServiceRegistry(cfg)
		assert.Error(t, err)
	})
}

func TestInitializeIntegrations_ComponentScoping(t *testing.T) {
	logger := slog.Default()
	tracer := noop.NewTracerProvider().Tracer("test")

	fullCfg := &Config{
		MultitenancyEnabled: true,
		ExtAuthzService: &ExtAuthzServiceConfig{
			JwkSetURL: "http://localhost:8080/.well-known/jwks.json",
		},
		OpenFGA: &OpenFGAConfig{
			Address:              "http://localhost:8081",
			StoreID:              "01HV8G7Z8B0000000000000000",
			AuthorizationModelID: "01HV8G7Z8B0000000000000001",
			ApiKey:               "secret-key",
		},
		Valkey: &ValkeyConfig{
			Enabled: false,
		},
		STS: &STSConfig{
			Address:              "localhost:9090",
			UseTLS:               false,
			Timeout:              time.Second,
			EagerConnectionCheck: false,
		},
		Postgres: &PostgresConfig{
			// Unreachable Postgres to prove non-Postgres commands don't dial Postgres!
			Host:           "127.0.0.1",
			Port:           1,
			User:           "authz",
			Password:       "authz",
			DBName:         "authz",
			SSLMode:        "disable",
			ConnectTimeout: 100 * time.Millisecond,
		},
		Kafka: &KafkaConfig{
			Enabled:                   true,
			Brokers:                   []string{"localhost:9092"},
			ConsumerGroup:             "authz-listener",
			FederatedServicesStrategy: "config",
			FederatedServices:         []string{"tenant-service"},
		},
	}

	t.Run("authz write-model initializes only OpenFGA and skips unreachable Postgres", func(t *testing.T) {
		integrations, err := InitializeIntegrations(fullCfg, logger, tracer, ComponentOpenFGAModelWriter)
		require.NoError(t, err)
		defer integrations.CleanupIntegrations(logger)

		assert.NotNil(t, integrations.OpenFGA)
		assert.Nil(t, integrations.Postgres)
		assert.Nil(t, integrations.Valkey)
		assert.Nil(t, integrations.STS)
		assert.Nil(t, integrations.KafkaConsumer)
	})

	t.Run("listen component list initializes Kafka without OpenFGA or STS", func(t *testing.T) {
		integrations, err := InitializeIntegrations(fullCfg, logger, tracer, ComponentKafka)
		require.NoError(t, err)
		defer integrations.CleanupIntegrations(logger)

		assert.NotNil(t, integrations.KafkaConsumer)
		assert.NotNil(t, integrations.ServiceRegistry)
		assert.Nil(t, integrations.OpenFGA)
		assert.Nil(t, integrations.STS)
		assert.Nil(t, integrations.Valkey)

		// Second call to InitKafkaConsumer is idempotent
		require.NoError(t, integrations.InitKafkaConsumer(fullCfg, logger))
	})

	t.Run("STS, Valkey noop, OpenFGA, and ExtAuthz initialize cleanly and build services", func(t *testing.T) {
		integrations, err := InitializeIntegrations(fullCfg, logger, tracer,
			ComponentOpenFGA,
			ComponentValkey,
			ComponentSTS,
			ComponentExtAuthz,
		)
		require.NoError(t, err)
		defer integrations.CleanupIntegrations(logger)

		assert.NotNil(t, integrations.OpenFGA)
		assert.NotNil(t, integrations.Valkey)
		assert.NotNil(t, integrations.STS)
		assert.Nil(t, integrations.Postgres)

		reg := metrics.NewRegistry()
		svcs := integrations.InitializeServices(tracer, logger, reg)
		assert.NotNil(t, svcs.Permissions)
		assert.NotNil(t, svcs.ExternalAuthz)
	})

	t.Run("disabled Kafka uses noop consumer", func(t *testing.T) {
		disabledKafkaCfg := *fullCfg
		disabledKafkaCfg.Kafka = &KafkaConfig{Enabled: false}

		integrations, err := InitializeIntegrations(&disabledKafkaCfg, logger, tracer, ComponentKafka)
		require.NoError(t, err)
		defer integrations.CleanupIntegrations(logger)

		assert.NotNil(t, integrations.KafkaConsumer)
		assert.NotNil(t, integrations.ServiceRegistry)
	})
}
