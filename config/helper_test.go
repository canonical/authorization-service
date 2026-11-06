// Copyright 2026 Canonical Ltd.
// SPDX-License-Identifier: AGPL-3.0-only

package config

import (
	"context"
	"io"
	"log/slog"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/trace/noop"
)

func TestBuildServiceRegistry_Strategies(t *testing.T) {
	t.Run("strategy fs uses ModelFS scanning excluding dummy and core", func(t *testing.T) {
		cfg := &ListenerConfig{
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
		cfg := &ListenerConfig{
			FederatedServicesStrategy: "config",
			FederatedServices:         []string{"payments", "invoicing"},
		}
		reg, err := BuildServiceRegistry(cfg)
		require.NoError(t, err)
		assert.Equal(t, []string{"payments", "invoicing"}, reg.Slugs())
	})

	t.Run("strategy auto with empty slice falls back to FS scanning", func(t *testing.T) {
		cfg := &ListenerConfig{
			FederatedServicesStrategy: "auto",
			FederatedServices:         nil,
		}
		reg, err := BuildServiceRegistry(cfg)
		require.NoError(t, err)
		assert.Contains(t, reg.Slugs(), "tenant-service")
		assert.NotContains(t, reg.Slugs(), "dummy")
		assert.NotContains(t, reg.Slugs(), "core")
	})

	t.Run("strategy auto with non-empty slice uses config slice", func(t *testing.T) {
		cfg := &ListenerConfig{
			FederatedServicesStrategy: "auto",
			FederatedServices:         []string{"billing"},
		}
		reg, err := BuildServiceRegistry(cfg)
		require.NoError(t, err)
		assert.Equal(t, []string{"billing"}, reg.Slugs())
	})

	t.Run("unknown strategy returns error", func(t *testing.T) {
		cfg := &ListenerConfig{
			FederatedServicesStrategy: "invalid",
		}
		_, err := BuildServiceRegistry(cfg)
		assert.Error(t, err)
	})
}

func TestInitServiceRegistry_And_NewOIDCVerifier(t *testing.T) {
	integrations := &Integrations{}
	cfg := &Config{
		Listener: &ListenerConfig{
			FederatedServicesStrategy: "config",
			FederatedServices:         []string{"payments"},
		},
	}

	require.NoError(t, integrations.InitServiceRegistry(cfg))
	require.NotNil(t, integrations.ServiceRegistry)
	assert.Equal(t, []string{"payments"}, integrations.ServiceRegistry.Slugs())

	verifier := NewOIDCVerifier(context.Background(), "http://localhost:8080/.well-known/jwks.json")
	assert.NotNil(t, verifier)
}

func TestInitializeIntegrations_ComponentScoped(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	tracer := noop.NewTracerProvider().Tracer("test")

	// GIVEN a Config populated with STS (eager check on invalid port), OpenFGA, and Valkey,
	// but validated ONLY for ComponentListener (standalone mode)
	cfg := &Config{
		Listener: &ListenerConfig{
			Mode:                      ListenModeStandalone,
			FederatedServicesStrategy: "auto",
		},
		ExtAuthzService: &ExtAuthzServiceConfig{
			JwkSetURL: "http://localhost:8080/.well-known/jwks.json",
		},
		OpenFGA: &OpenFGAConfig{
			Address: "http://localhost:8081",
			StoreID: "01J00000000000000000000000",
			ApiKey:  "secret",
		},
		Valkey: &ValkeyConfig{
			Enabled: false,
		},
		STS: &STSConfig{
			Address:              "127.0.0.1:1",
			EagerConnectionCheck: true,
		},
	}
	require.NoError(t, cfg.ValidateComponents(ComponentListener))

	// WHEN InitializeIntegrations is called
	integrations, err := InitializeIntegrations(cfg, logger, tracer)
	require.NoError(t, err)
	defer integrations.CleanupIntegrations(logger)

	// THEN OpenFGA, Valkey, and STS are NOT initialized because their components were not requested
	assert.Nil(t, integrations.OpenFGA)
	assert.Nil(t, integrations.Valkey)
	assert.Nil(t, integrations.STS)
	assert.Equal(t, "http://localhost:8080/.well-known/jwks.json", integrations.jwkSetUrl)
}
