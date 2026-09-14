// Copyright 2026 Canonical Ltd.
// SPDX-License-Identifier: AGPL-3.0-only

package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupRequiredEnv(t *testing.T) {
	t.Setenv("SERVER_DEVELOPMENT", "true")
	t.Setenv("OPENFGA_STORE_ID", "test-store")
	t.Setenv("OPENFGA_AUTHORIZATION_MODEL_ID", "test-model")
	t.Setenv("OPENFGA_API_KEY", "test-key")
}

func TestLoadConfig_Defaults(t *testing.T) {
	setupRequiredEnv(t)

	// GIVEN a mock cobra command with no flags set
	cmd := &cobra.Command{}
	cmd.Flags().String("config", "", "")

	// WHEN loading the configuration
	cfg, err := LoadConfig(cmd)

	// THEN it should load the default values successfully
	require.NoError(t, err)
	assert.Equal(t, 9091, cfg.Server.GRPCPort)
	assert.Equal(t, "0.0.0.0", cfg.Server.Host)
	assert.Equal(t, "info", cfg.Logging.Level)
	assert.Equal(t, false, cfg.Valkey.Enabled)
	assert.Equal(t, "authorization-service", cfg.Postgres.DBName)
	assert.Equal(t, "test-key", cfg.OpenFGA.ApiKey)
	assert.Equal(t, "test-store", cfg.OpenFGA.StoreID)
	assert.Equal(t, "test-model", cfg.OpenFGA.AuthorizationModelID)
}

func TestLoadConfig_File(t *testing.T) {
	setupRequiredEnv(t)

	// GIVEN a temporary YAML config file
	tempDir := t.TempDir()
	cfgPath := filepath.Join(tempDir, "config.yaml")

	yamlContent := `
server:
  grpc_port: 9999
  host: "127.0.0.1"
postgres:
  db_name: "custom_db"
`
	err := os.WriteFile(cfgPath, []byte(yamlContent), 0644)
	require.NoError(t, err)

	// AND a mock cobra command with --config set
	cmd := &cobra.Command{}
	cmd.Flags().String("config", cfgPath, "")
	_ = cmd.Flags().Set("config", cfgPath)

	// WHEN loading the configuration
	cfg, err := LoadConfig(cmd)

	// THEN it should load the values from the YAML file
	require.NoError(t, err)
	assert.Equal(t, 9999, cfg.Server.GRPCPort)
	assert.Equal(t, "127.0.0.1", cfg.Server.Host)
	assert.Equal(t, "custom_db", cfg.Postgres.DBName)
	// AND other fields should retain their defaults
	assert.Equal(t, "info", cfg.Logging.Level)
}

func TestLoadConfig_EnvOverride(t *testing.T) {
	setupRequiredEnv(t)

	// GIVEN environment variables set
	t.Setenv("SERVER_GRPC_PORT", "8888")
	t.Setenv("POSTGRES_DB_NAME", "env_db")
	t.Setenv("LOGGING_ADD_SOURCE", "true")

	cmd := &cobra.Command{}
	cmd.Flags().String("config", "", "")

	// WHEN loading the configuration
	cfg, err := LoadConfig(cmd)

	// THEN environment variables should override defaults
	require.NoError(t, err)
	assert.Equal(t, 8888, cfg.Server.GRPCPort)
	assert.Equal(t, "env_db", cfg.Postgres.DBName)
	assert.True(t, cfg.Logging.AddSource)
}

func TestLoadConfig_FlagsOverride(t *testing.T) {
	setupRequiredEnv(t)

	// GIVEN environment variables set AND flags explicitly passed
	t.Setenv("SERVER_GRPC_PORT", "8888") // Env value

	cmd := &cobra.Command{}
	cmd.Flags().String("config", "", "")
	cmd.Flags().Int("grpc-port", 7777, "")   // Flag value
	_ = cmd.Flags().Set("grpc-port", "7777") // Emulate explicit CLI passing

	// WHEN loading the configuration
	cfg, err := LoadConfig(cmd)

	// THEN the CLI flag should have the highest priority and override the Env var
	require.NoError(t, err)
	assert.Equal(t, 7777, cfg.Server.GRPCPort)
}

func TestLoadConfig_OpenFGAEnvAndFlags(t *testing.T) {
	setupRequiredEnv(t)

	t.Setenv("OPENFGA_API_KEY", "env-secret-token")
	t.Setenv("OPENFGA_STORE_ID", "env-store-123")
	t.Setenv("POSTGRES_DB", "env_postgres_db")
	t.Setenv("LOG_LEVEL", "debug")
	t.Setenv("DEV", "true")

	cmd := &cobra.Command{}
	cmd.Flags().String("config", "", "")
	cmd.Flags().String("fga-api-key", "", "")
	cmd.Flags().String("fga-store-id", "", "")

	// WHEN loading without setting flags
	cfg1, err := LoadConfig(cmd)
	require.NoError(t, err)
	assert.Equal(t, "env-secret-token", cfg1.OpenFGA.ApiKey)
	assert.Equal(t, "env-store-123", cfg1.OpenFGA.StoreID)
	assert.Equal(t, "env_postgres_db", cfg1.Postgres.DBName)
	assert.Equal(t, "debug", cfg1.Logging.Level)
	assert.True(t, cfg1.Server.Development)

	// WHEN explicitly setting CLI flags
	_ = cmd.Flags().Set("fga-api-key", "cli-override-token")
	_ = cmd.Flags().Set("fga-store-id", "cli-override-store")

	cfg2, err := LoadConfig(cmd)
	require.NoError(t, err)
	assert.Equal(t, "cli-override-token", cfg2.OpenFGA.ApiKey)
	assert.Equal(t, "cli-override-store", cfg2.OpenFGA.StoreID)
}

func TestLoadConfig_WithoutAuthorizationModelID(t *testing.T) {
	t.Setenv("SERVER_DEVELOPMENT", "true")
	t.Setenv("OPENFGA_STORE_ID", "test-store")
	t.Setenv("OPENFGA_API_KEY", "test-key")

	cmd := &cobra.Command{}
	cmd.Flags().String("config", "", "")

	cfg, err := LoadConfig(cmd)
	require.NoError(t, err)
	assert.Equal(t, "", cfg.OpenFGA.AuthorizationModelID)
}
