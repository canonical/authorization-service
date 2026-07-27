// Copyright 2026 Canonical Ltd.
// SPDX-License-Identifier: AGPL-3.0-only

package config

import (
	"fmt"
	"strings"
	"time"

	"github.com/go-viper/mapstructure/v2"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"github.com/spf13/viper"
)

// LoadConfig loads the configuration from default values, config file, environment variables, and CLI flags.
func LoadConfig(cmd *cobra.Command) (*Config, error) {
	v := viper.New()

	// 1. Set Defaults
	setViperDefaults(v)

	// 2. Bind Environment Variables automatically using a key replacer
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	v.AutomaticEnv()

	// 3. Bind CLI flags (only if explicitly changed by the user)
	if cmd != nil {
		bindFlags(v, cmd)
	}

	// 4. Load from File
	var cfgFile string
	if cmd != nil {
		cfgFile, _ = cmd.Flags().GetString("config")
	}

	if cfgFile != "" {
		v.SetConfigFile(cfgFile)
	} else {
		v.AddConfigPath(".")
		v.AddConfigPath("/etc/authz")
		v.SetConfigName("cerberus")
		v.SetConfigType("yaml")
	}

	if err := v.ReadInConfig(); err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); !ok || cfgFile != "" {
			return nil, fmt.Errorf("failed to read config file: %w", err)
		}
	}

	// 5. Unmarshal with mapstructure hooks to support slices and durations
	cfg := &Config{}
	if err := v.Unmarshal(cfg, func(config *mapstructure.DecoderConfig) {
		config.TagName = "mapstructure"
		config.DecodeHook = mapstructure.ComposeDecodeHookFunc(
			mapstructure.StringToSliceHookFunc(","),
			mapstructure.StringToTimeDurationHookFunc(),
		)
	}); err != nil {
		return nil, fmt.Errorf("failed to unmarshal configuration: %w", err)
	}

	// 6. Perform Struct Validation
	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("configuration validation failed: %w", err)
	}

	return cfg, nil
}

// setViperDefaults sets default values for all Viper configuration paths
func setViperDefaults(v *viper.Viper) {
	// Server
	v.SetDefault("server.grpc_port", 9091)
	v.SetDefault("server.http_port", 8070)
	v.SetDefault("server.host", "0.0.0.0")
	v.SetDefault("server.shutdown_timeout", 15*time.Second)
	v.SetDefault("server.development", false)

	// ExtAuthzService
	v.SetDefault("ext_authz_service.jwk_set_url", "http://localhost:8080/.well-known/jwks.json")

	// OpenFGA
	v.SetDefault("open_fga.address", "http://localhost:8081")
	v.SetDefault("open_fga.timeout", 10*time.Second)
	v.SetDefault("open_fga.store_id", "")
	v.SetDefault("open_fga.authorization_model_id", "")
	v.SetDefault("open_fga.api_key", "")

	// Valkey
	v.SetDefault("valkey.enabled", false)
	v.SetDefault("valkey.address", "localhost:6379")
	v.SetDefault("valkey.db", 0)
	v.SetDefault("valkey.pool_size", 10)
	v.SetDefault("valkey.timeout", 5*time.Second)
	v.SetDefault("valkey.use_tls", false)

	// Postgres
	v.SetDefault("postgres.host", "localhost")
	v.SetDefault("postgres.port", 5432)
	v.SetDefault("postgres.user", "authz")
	v.SetDefault("postgres.password", "authz-password")
	v.SetDefault("postgres.db_name", "cerberus")
	v.SetDefault("postgres.ssl_mode", "disable")
	v.SetDefault("postgres.max_open_conns", 25)
	v.SetDefault("postgres.max_idle_conns", 5)
	v.SetDefault("postgres.conn_max_lifetime", 30*time.Minute)
	v.SetDefault("postgres.conn_max_idle_time", 5*time.Minute)
	v.SetDefault("postgres.connect_timeout", 10*time.Second)

	// STS
	v.SetDefault("sts.address", "localhost:9090")
	v.SetDefault("sts.use_tls", false)
	v.SetDefault("sts.timeout", 10*time.Second)
	v.SetDefault("sts.eager_connection_check", false)

	// Kafka
	v.SetDefault("kafka.enabled", false)
	v.SetDefault("kafka.brokers", []string{"localhost:9092"})
	v.SetDefault("kafka.consumer_group", "authz-listener")
	v.SetDefault("kafka.topic_partitions", 1)
	v.SetDefault("kafka.topic_replication_factor", 1)

	// Worker
	v.SetDefault("worker.enabled", false)
	v.SetDefault("worker.batch_size", 100)
	v.SetDefault("worker.poll_interval", 1*time.Second)
	v.SetDefault("worker.max_attempts", 5)
	v.SetDefault("worker.retry_backoff", 5*time.Minute)
	v.SetDefault("worker.stale_timeout", 15*time.Minute)
	v.SetDefault("worker.reaper_interval", 1*time.Minute)

	// Logging
	v.SetDefault("logging.level", "info")
	v.SetDefault("logging.format", "json")

	// Telemetry
	v.SetDefault("telemetry.enabled", false)
	v.SetDefault("telemetry.service_name", "authorization-service")
	v.SetDefault("telemetry.service_version", "v1.0.0")
}

// bindFlags binds standard Cobra flags to Viper keys if explicitly set
func bindFlags(v *viper.Viper, cmd *cobra.Command) {
	flags := cmd.Flags()

	bindFlagIfChanged(v, flags, "grpc-port", "server.grpc_port")
	bindFlagIfChanged(v, flags, "http-port", "server.http_port")
	bindFlagIfChanged(v, flags, "server-host", "server.host")
	bindFlagIfChanged(v, flags, "db-host", "postgres.host")
	bindFlagIfChanged(v, flags, "db-port", "postgres.port")
	bindFlagIfChanged(v, flags, "db-name", "postgres.db_name")
	bindFlagIfChanged(v, flags, "db-user", "postgres.user")
	bindFlagIfChanged(v, flags, "dev", "server.development")
	bindFlagIfChanged(v, flags, "log-level", "logging.level")
	bindFlagIfChanged(v, flags, "log-format", "logging.format")
	bindFlagIfChanged(v, flags, "fga-address", "open_fga.address")
	bindFlagIfChanged(v, flags, "fga-store-id", "open_fga.store_id")
	bindFlagIfChanged(v, flags, "fga-model-id", "open_fga.authorization_model_id")
}

// bindFlagIfChanged binds a pflag to a Viper key only if the flag was explicitly changed by the user
func bindFlagIfChanged(v *viper.Viper, flags *pflag.FlagSet, flagName string, viperKey string) {
	if flags.Lookup(flagName) != nil && flags.Changed(flagName) {
		_ = v.BindPFlag(viperKey, flags.Lookup(flagName))
	}
}
