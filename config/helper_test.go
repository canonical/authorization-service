// Copyright 2026 Canonical Ltd.
// SPDX-License-Identifier: AGPL-3.0-only

package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBuildServiceRegistry_Strategies(t *testing.T) {
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
