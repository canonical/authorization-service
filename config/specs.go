// Copyright 2026 Canonical Ltd.
// SPDX-License-Identifier: AGPL-3.0-only

package config

import (
	"context"
	"crypto/tls"
	"fmt"
	"log/slog"
	"os"
	"time"

	"github.com/go-playground/validator/v10"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.4.0"
	"go.opentelemetry.io/otel/trace"
	"google.golang.org/grpc"
	"google.golang.org/grpc/connectivity"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
)

// Config represents the application configuration
type Config struct {
	MultitenancyEnabled bool                   `validate:"" envconfig:"MULTITENANCY_ENABLED" mapstructure:"multitenancy_enabled" default:"false"`
	Server              *ServerConfig          `validate:"required" mapstructure:"server"`
	ExtAuthzService     *ExtAuthzServiceConfig `validate:"required" mapstructure:"ext_authz_service"`
	OpenFGA             *OpenFGAConfig         `validate:"required" mapstructure:"open_fga"`
	Valkey              *ValkeyConfig          `validate:"required" mapstructure:"valkey"`
	STS                 *STSConfig             `validate:"required" mapstructure:"sts"`
	Postgres            *PostgresConfig        `validate:"required" mapstructure:"postgres"`
	Logging             *LoggingConfig         `validate:"required" mapstructure:"logging"`
	Telemetry           *TelemetryConfig       `validate:"required" mapstructure:"telemetry"`
	Kafka               *KafkaConfig           `validate:"required" mapstructure:"kafka"`
	Worker              *WorkerConfig          `validate:"required" mapstructure:"worker"`
}

// ServerConfig contains server configuration
type ServerConfig struct {
	GRPCPort                int           `validate:"required,min=1,max=65535" envconfig:"GRPC_PORT" mapstructure:"grpc_port" default:"9091"`
	HTTPPort                int           `validate:"required,min=1,max=65535" envconfig:"HTTP_PORT" mapstructure:"http_port" default:"8070"`
	Host                    string        `validate:"required" envconfig:"SERVER_HOST" mapstructure:"host" default:"0.0.0.0"`
	GracefulShutdownTimeout time.Duration `validate:"" envconfig:"SERVER_SHUTDOWN_TIMEOUT" mapstructure:"shutdown_timeout" default:"15s"`
	Development             bool          `validate:"required" envconfig:"DEV" mapstructure:"development" default:"false"`
}

type ExtAuthzServiceConfig struct {
	JwkSetURL string `validate:"required,http_url" envconfig:"EXTAUTHZ_JWK_SET_URL" mapstructure:"jwk_set_url" default:"http://localhost:8080/.well-known/jwks.json"`
}

// GetGRPCAddress returns the full gRPC server address
func (s *ServerConfig) GetGRPCAddress() string {
	return fmt.Sprintf("%s:%d", s.Host, s.GRPCPort)
}

// GetHTTPAddress returns the full HTTP server address
func (s *ServerConfig) GetHTTPAddress() string {
	return fmt.Sprintf("%s:%d", s.Host, s.HTTPPort)
}

// OpenFGAConfig contains OpenFGA configuration
type OpenFGAConfig struct {
	Address              string        `validate:"required" envconfig:"OPENFGA_ADDRESS" mapstructure:"address" default:"http://localhost:8081"`
	StoreID              string        `validate:"required" envconfig:"OPENFGA_STORE_ID" mapstructure:"store_id"`
	AuthorizationModelID string        `validate:"required" envconfig:"OPENFGA_AUTHZ_MODEL_ID" mapstructure:"authorization_model_id"`
	ApiKey               string        `validate:"required" envconfig:"OPENFGA_API_KEY" mapstructure:"api_key"`
	Timeout              time.Duration `validate:"" envconfig:"OPENFGA_TIMEOUT" mapstructure:"timeout" default:"10s"`
}

// ValkeyConfig contains Valkey (Redis-compatible) configuration
type ValkeyConfig struct {
	Enabled  bool          `validate:"" envconfig:"VALKEY_ENABLED" mapstructure:"enabled" default:"false"`
	Address  string        `validate:"required_if=Enabled true" envconfig:"VALKEY_ADDRESS" mapstructure:"address" default:"localhost:6379"`
	Username string        `validate:"" envconfig:"VALKEY_USERNAME" mapstructure:"username"`
	Password string        `validate:"" envconfig:"VALKEY_PASSWORD" mapstructure:"password"`
	DB       int           `validate:"min=0,max=15" envconfig:"VALKEY_DB" mapstructure:"db" default:"0"`
	PoolSize int           `validate:"min=1" envconfig:"VALKEY_POOL_SIZE" mapstructure:"pool_size" default:"10"`
	Timeout  time.Duration `validate:"" envconfig:"VALKEY_TIMEOUT" mapstructure:"timeout" default:"5s"`
	UseTLS   bool          `validate:"" envconfig:"VALKEY_USE_TLS" mapstructure:"use_tls" default:"false"`
}

// PostgresConfig contains PostgreSQL database configuration.
// Postgres 14 or later is required.
type PostgresConfig struct {
	Host            string        `validate:"required" envconfig:"POSTGRES_HOST" mapstructure:"host" default:"localhost"`
	Port            int           `validate:"required,min=1,max=65535" envconfig:"POSTGRES_PORT" mapstructure:"port" default:"5432"`
	User            string        `validate:"required" envconfig:"POSTGRES_USER" mapstructure:"user" default:"authz"`
	Password        string        `validate:"" envconfig:"POSTGRES_PASSWORD" mapstructure:"password" default:"authz-password"`
	DBName          string        `validate:"required" envconfig:"POSTGRES_DB" mapstructure:"db_name" default:"cerberus"`
	SSLMode         string        `validate:"required,oneof=disable require verify-ca verify-full" envconfig:"POSTGRES_SSL_MODE" mapstructure:"ssl_mode" default:"disable"`
	MaxOpenConns    int32         `validate:"min=1" envconfig:"POSTGRES_MAX_OPEN_CONNS" mapstructure:"max_open_conns" default:"25"`
	MaxIdleConns    int32         `validate:"min=1" envconfig:"POSTGRES_MAX_IDLE_CONNS" mapstructure:"max_idle_conns" default:"5"`
	ConnMaxLifetime time.Duration `validate:"" envconfig:"POSTGRES_CONN_MAX_LIFETIME" mapstructure:"conn_max_lifetime" default:"30m"`
	ConnMaxIdleTime time.Duration `validate:"" envconfig:"POSTGRES_CONN_MAX_IDLE_TIME" mapstructure:"conn_max_idle_time" default:"5m"`
	ConnectTimeout  time.Duration `validate:"" envconfig:"POSTGRES_CONNECT_TIMEOUT" mapstructure:"connect_timeout" default:"10s"`
}

func (c *LoggingConfig) SetupLogger() *slog.Logger {
	var handler slog.Handler
	opts := &slog.HandlerOptions{
		Level: ParseLogLevel(c.Level),
	}
	if c.Format == "json" {
		handler = slog.NewJSONHandler(os.Stdout, opts)
	} else {
		handler = slog.NewTextHandler(os.Stdout, opts)
	}
	return slog.New(handler)
}

// STSConfig contains Secure Token Service configuration
type STSConfig struct {
	Address              string        `validate:"required" envconfig:"STS_ADDRESS" mapstructure:"address" default:"localhost:9090"`
	UseTLS               bool          `validate:"" envconfig:"STS_USE_TLS" mapstructure:"use_tls" default:"false"`
	Timeout              time.Duration `validate:"" envconfig:"STS_TIMEOUT" mapstructure:"timeout" default:"10s"`
	EagerConnectionCheck bool          `validate:"" envconfig:"STS_EAGER_CONNECTION_CHECK" mapstructure:"eager_connection_check" default:"false"`
}

// CreateSTSConnection creates a gRPC connection to the STS service
func (s *STSConfig) CreateSTSConnection() (*grpc.ClientConn, error) {
	var (
		opts           []grpc.DialOption
		credentialsOpt grpc.DialOption
	)

	if s.UseTLS {
		credentialsOpt = grpc.WithTransportCredentials(
			credentials.NewTLS(
				&tls.Config{
					MinVersion: tls.VersionTLS12,
				},
			),
		)
	} else {
		credentialsOpt = grpc.WithTransportCredentials(insecure.NewCredentials())
	}

	opts = append(opts, credentialsOpt)

	conn, err := grpc.NewClient(s.Address, opts...)
	if err != nil {
		return nil, fmt.Errorf("failed to create STS connection for %s: %w", s.Address, err)
	}

	if s.EagerConnectionCheck {
		err = s.waitForConnectionReady(conn)
		if err != nil {
			return nil, err
		}
	}

	return conn, nil
}

func (s *STSConfig) waitForConnectionReady(conn *grpc.ClientConn) error {
	conn.Connect()

	dialCtx, cancel := context.WithTimeout(context.Background(), s.Timeout)
	defer cancel()

	for {
		state := conn.GetState()
		if state == connectivity.Ready {
			break
		}

		if !conn.WaitForStateChange(dialCtx, state) {
			_ = conn.Close()
			return fmt.Errorf("timeout waiting for STS connection to %s: %w", s.Address, dialCtx.Err())
		}
	}

	return nil
}

// KafkaConfig contains Kafka consumer configuration for the permission-update
// ingestion listener.
//
// FederatedServices is the registry source: each entry is a service slug, and
// the listener subscribes to the derived "<slug>.permissions" topic within a
// single consumer group. Topics are never hardcoded.
type KafkaConfig struct {
	Enabled                bool     `validate:"" envconfig:"KAFKA_ENABLED" mapstructure:"enabled" default:"false"`
	Brokers                []string `validate:"required_if=Enabled true" envconfig:"KAFKA_BROKERS" mapstructure:"brokers" default:"localhost:9092"`
	FederatedServices      []string `validate:"required_if=Enabled true" envconfig:"FEDERATED_SERVICES" mapstructure:"federated_services"`
	ConsumerGroup          string   `validate:"required_if=Enabled true" envconfig:"KAFKA_CONSUMER_GROUP" mapstructure:"consumer_group" default:"authz-listener"`
	TopicPartitions        int      `validate:"min=1" envconfig:"KAFKA_TOPIC_PARTITIONS" mapstructure:"topic_partitions" default:"1"`
	TopicReplicationFactor int      `validate:"min=1" envconfig:"KAFKA_TOPIC_REPLICATION_FACTOR" mapstructure:"topic_replication_factor" default:"1"`
}

// WorkerConfig contains configuration for the async permission-update worker
// that claims rows from permission_update_work and applies them to OpenFGA.
//
// The worker processes rows serially and scales horizontally by running multiple
// instances (FOR UPDATE SKIP LOCKED assigns disjoint rows), mirroring how the
// listener scales.
type WorkerConfig struct {
	Enabled        bool          `validate:"" envconfig:"WORKER_ENABLED" mapstructure:"enabled" default:"false"`
	BatchSize      int           `validate:"min=1" envconfig:"WORKER_BATCH_SIZE" mapstructure:"batch_size" default:"100"`
	PollInterval   time.Duration `validate:"" envconfig:"WORKER_POLL_INTERVAL" mapstructure:"poll_interval" default:"1s"`
	MaxAttempts    int           `validate:"min=1" envconfig:"WORKER_MAX_ATTEMPTS" mapstructure:"max_attempts" default:"5"`
	RetryBackoff   time.Duration `validate:"" envconfig:"WORKER_RETRY_BACKOFF" mapstructure:"retry_backoff" default:"5m"`
	StaleTimeout   time.Duration `validate:"" envconfig:"WORKER_STALE_TIMEOUT" mapstructure:"stale_timeout" default:"15m"`
	ReaperInterval time.Duration `validate:"" envconfig:"WORKER_REAPER_INTERVAL" mapstructure:"reaper_interval" default:"1m"`
}

// LoggingConfig contains logging configuration
type LoggingConfig struct {
	Level  string `validate:"required,oneof=debug info warn error" envconfig:"LOG_LEVEL" mapstructure:"level" default:"info"`
	Format string `validate:"required,oneof=json text" envconfig:"LOG_FORMAT" mapstructure:"format" default:"json"`
}

// TelemetryConfig contains telemetry configuration
type TelemetryConfig struct {
	Enabled        bool   `validate:"" envconfig:"TELEMETRY_ENABLED" mapstructure:"enabled" default:"false"`
	OTLPEndpoint   string `validate:"" envconfig:"OTEL_EXPORTER_OTLP_ENDPOINT" mapstructure:"otlp_endpoint"`
	ServiceName    string `validate:"required_if=Enabled true" envconfig:"OTEL_SERVICE_NAME" mapstructure:"service_name" default:"authorization-service"`
	ServiceVersion string `validate:"required_if=Enabled true" envconfig:"OTEL_SERVICE_VERSION" mapstructure:"service_version" default:"v1.0.0"`
}

func (t *TelemetryConfig) SetupTelemetry(ctx context.Context, logger *slog.Logger) (trace.Tracer, func(context.Context) error, error) {
	if !t.Enabled {
		logger.Info("Telemetry is disabled")
		return otel.Tracer(t.ServiceName), func(context.Context) error { return nil }, nil
	}

	res, err := resource.New(ctx,
		resource.WithAttributes(
			semconv.ServiceNameKey.String(t.ServiceName),
			semconv.ServiceVersionKey.String(t.ServiceVersion),
		),
	)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create resource: %w", err)
	}

	traceExporter, err := otlptrace.New(ctx,
		otlptracegrpc.NewClient(
			otlptracegrpc.WithEndpoint(t.OTLPEndpoint),
			otlptracegrpc.WithInsecure(), // WithTLSCredentials()
		),
	)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create trace exporter: %w", err)
	}

	traceProvider := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(traceExporter),
		sdktrace.WithResource(res),
		sdktrace.WithSampler(sdktrace.AlwaysSample()),
	)

	otel.SetTracerProvider(traceProvider)

	tracer := traceProvider.Tracer(t.ServiceName)
	shutdown := func(ctx context.Context) error {
		if err := traceProvider.Shutdown(ctx); err != nil {
			return fmt.Errorf("failed to shutdown trace provider: %w", err)
		}

		return nil
	}

	return tracer, shutdown, nil
}

func (c *Config) Validate() error {
	validate := validator.New()
	if err := validate.Struct(c); err != nil {
		return fmt.Errorf("configuration validation failed: %w", err)
	}

	return nil
}

// ParseLogLevel converts a string representation of a log level to slog.Level.
func ParseLogLevel(level string) slog.Level {
	switch level {
	case "debug":
		return slog.LevelDebug
	case "info":
		return slog.LevelInfo
	case "warn":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}
