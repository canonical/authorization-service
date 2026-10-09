// Copyright 2026 Canonical Ltd.
// SPDX-License-Identifier: AGPL-3.0-only

package config

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/go-viper/mapstructure/v2"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"github.com/spf13/viper"
)

// ErrConfigLoad wraps every error LoadConfig returns, so callers can distinguish a
// config-load failure from errors already logged by a command that got further than LoadConfig.
var ErrConfigLoad = errors.New("failed to load configuration")

// LoadConfig loads the full configuration and performs validation across all components.
func LoadConfig(cmd *cobra.Command) (*Config, error) {
	return LoadConfigFor(cmd, AllComponents...)
}

// LoadConfigFor loads configuration and performs component-scoped validation for specified components.
func LoadConfigFor(cmd *cobra.Command, components ...Component) (*Config, error) {
	v := viper.New()

	// 1. Set Defaults
	setViperDefaults(v)

	// 2. Bind Environment Variables automatically using a key replacer
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	v.AutomaticEnv()
	bindEnvVars(v)

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
		v.SetConfigName("authorization-service")
		v.SetConfigType("yaml")
	}

	if err := v.ReadInConfig(); err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); !ok || cfgFile != "" {
			return nil, fmt.Errorf("%w: failed to read config file: %w", ErrConfigLoad, err)
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
		return nil, fmt.Errorf("%w: failed to unmarshal configuration: %w", ErrConfigLoad, err)
	}

	// 6. Perform Component-Scoped Struct Validation
	if err := cfg.ValidateComponents(components...); err != nil {
		return nil, fmt.Errorf("%w: configuration validation failed: %w", ErrConfigLoad, err)
	}

	return cfg, nil
}

// setViperDefaults sets default values for all Viper configuration paths
func setViperDefaults(v *viper.Viper) {
	// Multitenancy
	v.SetDefault("multitenancy_enabled", false)

	// Server
	v.SetDefault("server.grpc_port", 9091)
	v.SetDefault("server.http_port", 8070)
	v.SetDefault("server.host", "0.0.0.0")
	v.SetDefault("server.shutdown_timeout", 15*time.Second)
	v.SetDefault("server.development", false)

	// ExtAuthzService
	v.SetDefault("ext_authz_service.jwk_set_url", "http://localhost:8080/.well-known/jwks.json")

	// OpenFGA
	v.SetDefault("openfga.address", "http://localhost:8081")
	v.SetDefault("openfga.timeout", 10*time.Second)
	v.SetDefault("openfga.store_id", "")
	v.SetDefault("openfga.authorization_model_id", "")
	v.SetDefault("openfga.api_key", "")

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
	v.SetDefault("postgres.db_name", "authorization-service")
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
	v.SetDefault("kafka.federated_services_strategy", "auto")
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
	v.SetDefault("logging.add_source", false)

	// Telemetry
	v.SetDefault("telemetry.enabled", false)
	v.SetDefault("telemetry.service_name", "authorization-service")
	v.SetDefault("telemetry.service_version", "v1.0.0")

	// Metrics
	v.SetDefault("metrics.enabled", true)
	v.SetDefault("metrics.port", 9100)
	v.SetDefault("metrics.path", "/metrics")
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
	bindFlagIfChanged(v, flags, "fga-address", "openfga.address")
	bindFlagIfChanged(v, flags, "fga-store-id", "openfga.store_id")
	bindFlagIfChanged(v, flags, "fga-model-id", "openfga.authorization_model_id")
	bindFlagIfChanged(v, flags, "fga-api-key", "openfga.api_key")
}

// bindFlagIfChanged binds a pflag to a Viper key only if the flag was explicitly changed by the user
func bindFlagIfChanged(v *viper.Viper, flags *pflag.FlagSet, flagName string, viperKey string) {
	if flags.Lookup(flagName) != nil && flags.Changed(flagName) {
		_ = v.BindPFlag(viperKey, flags.Lookup(flagName))
	}
}

// bindEnvVars explicitly registers environment variable keys and aliases with Viper so
// that v.Unmarshal can bind environment variables into struct fields reliably.
func bindEnvVars(v *viper.Viper) {
	// Multitenancy
	_ = v.BindEnv("multitenancy_enabled", "MULTITENANCY_ENABLED")

	// Server
	_ = v.BindEnv("server.grpc_port", "GRPC_PORT", "SERVER_GRPC_PORT")
	_ = v.BindEnv("server.http_port", "HTTP_PORT", "SERVER_HTTP_PORT")
	_ = v.BindEnv("server.host", "SERVER_HOST")
	_ = v.BindEnv("server.shutdown_timeout", "SERVER_SHUTDOWN_TIMEOUT")
	_ = v.BindEnv("server.development", "DEV", "SERVER_DEVELOPMENT")

	// ExtAuthzService
	_ = v.BindEnv("ext_authz_service.jwk_set_url", "EXTAUTHZ_JWK_SET_URL", "EXT_AUTHZ_SERVICE_JWK_SET_URL")

	// OpenFGA
	_ = v.BindEnv("openfga.address", "OPENFGA_ADDRESS")
	_ = v.BindEnv("openfga.store_id", "OPENFGA_STORE_ID")
	_ = v.BindEnv("openfga.authorization_model_id", "OPENFGA_AUTHORIZATION_MODEL_ID")
	_ = v.BindEnv("openfga.api_key", "OPENFGA_API_KEY")
	_ = v.BindEnv("openfga.timeout", "OPENFGA_TIMEOUT")

	// Valkey
	_ = v.BindEnv("valkey.enabled", "VALKEY_ENABLED")
	_ = v.BindEnv("valkey.address", "VALKEY_ADDRESS")
	_ = v.BindEnv("valkey.username", "VALKEY_USERNAME")
	_ = v.BindEnv("valkey.password", "VALKEY_PASSWORD")
	_ = v.BindEnv("valkey.db", "VALKEY_DB")
	_ = v.BindEnv("valkey.pool_size", "VALKEY_POOL_SIZE")
	_ = v.BindEnv("valkey.timeout", "VALKEY_TIMEOUT")
	_ = v.BindEnv("valkey.use_tls", "VALKEY_USE_TLS")

	// Postgres
	_ = v.BindEnv("postgres.host", "POSTGRES_HOST")
	_ = v.BindEnv("postgres.port", "POSTGRES_PORT")
	_ = v.BindEnv("postgres.user", "POSTGRES_USER")
	_ = v.BindEnv("postgres.password", "POSTGRES_PASSWORD")
	_ = v.BindEnv("postgres.db_name", "POSTGRES_DB", "POSTGRES_DB_NAME")
	_ = v.BindEnv("postgres.ssl_mode", "POSTGRES_SSL_MODE")
	_ = v.BindEnv("postgres.max_open_conns", "POSTGRES_MAX_OPEN_CONNS")
	_ = v.BindEnv("postgres.max_idle_conns", "POSTGRES_MAX_IDLE_CONNS")
	_ = v.BindEnv("postgres.conn_max_lifetime", "POSTGRES_CONN_MAX_LIFETIME")
	_ = v.BindEnv("postgres.conn_max_idle_time", "POSTGRES_CONN_MAX_IDLE_TIME")
	_ = v.BindEnv("postgres.connect_timeout", "POSTGRES_CONNECT_TIMEOUT")

	// STS
	_ = v.BindEnv("sts.address", "STS_ADDRESS")
	_ = v.BindEnv("sts.use_tls", "STS_USE_TLS")
	_ = v.BindEnv("sts.timeout", "STS_TIMEOUT")
	_ = v.BindEnv("sts.eager_connection_check", "STS_EAGER_CONNECTION_CHECK")

	// Kafka
	_ = v.BindEnv("kafka.enabled", "KAFKA_ENABLED")
	_ = v.BindEnv("kafka.brokers", "KAFKA_BROKERS")
	_ = v.BindEnv("kafka.federated_services", "FEDERATED_SERVICES", "KAFKA_FEDERATED_SERVICES")
	_ = v.BindEnv("kafka.federated_services_strategy", "FEDERATED_SERVICES_STRATEGY", "KAFKA_FEDERATED_SERVICES_STRATEGY")
	_ = v.BindEnv("kafka.consumer_group", "KAFKA_CONSUMER_GROUP")
	_ = v.BindEnv("kafka.topic_partitions", "KAFKA_TOPIC_PARTITIONS")
	_ = v.BindEnv("kafka.topic_replication_factor", "KAFKA_TOPIC_REPLICATION_FACTOR")

	// Worker
	_ = v.BindEnv("worker.enabled", "WORKER_ENABLED")
	_ = v.BindEnv("worker.batch_size", "WORKER_BATCH_SIZE")
	_ = v.BindEnv("worker.poll_interval", "WORKER_POLL_INTERVAL")
	_ = v.BindEnv("worker.max_attempts", "WORKER_MAX_ATTEMPTS")
	_ = v.BindEnv("worker.retry_backoff", "WORKER_RETRY_BACKOFF")
	_ = v.BindEnv("worker.stale_timeout", "WORKER_STALE_TIMEOUT")
	_ = v.BindEnv("worker.reaper_interval", "WORKER_REAPER_INTERVAL")

	// Logging
	_ = v.BindEnv("logging.level", "LOG_LEVEL", "LOGGING_LEVEL")
	_ = v.BindEnv("logging.format", "LOG_FORMAT", "LOGGING_FORMAT")
	_ = v.BindEnv("logging.add_source", "LOG_ADD_SOURCE", "LOGGING_ADD_SOURCE")

	// Telemetry
	_ = v.BindEnv("telemetry.enabled", "TELEMETRY_ENABLED")
	_ = v.BindEnv("telemetry.otlp_endpoint", "OTEL_EXPORTER_OTLP_ENDPOINT", "TELEMETRY_OTLP_ENDPOINT")
	_ = v.BindEnv("telemetry.service_name", "OTEL_SERVICE_NAME", "TELEMETRY_SERVICE_NAME")
	_ = v.BindEnv("telemetry.service_version", "OTEL_SERVICE_VERSION", "TELEMETRY_SERVICE_VERSION")

	// Metrics
	_ = v.BindEnv("metrics.enabled", "METRICS_ENABLED")
	_ = v.BindEnv("metrics.port", "METRICS_PORT")
	_ = v.BindEnv("metrics.path", "METRICS_PATH")
}
