// Copyright 2026 Canonical Ltd.
// SPDX-License-Identifier: AGPL-3.0-only

package config

import (
	"context"
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

	"github.com/canonical/authorization-service/internal/integration/sts"
	"github.com/canonical/authorization-service/internal/logging"
)

type Component string

const (
	ComponentServer              Component = "server"
	ComponentExtAuthz            Component = "ext_authz_service"
	ComponentOpenFGA             Component = "openfga"
	ComponentOpenFGAModelWriter Component = "openfga_model_writer"
	ComponentValkey              Component = "valkey"
	ComponentSTS                 Component = "sts"
	ComponentPostgres            Component = "postgres"
	ComponentLogging             Component = "logging"
	ComponentTelemetry           Component = "telemetry"
	ComponentKafka               Component = "kafka"
	ComponentWorker              Component = "worker"
	ComponentMetrics             Component = "metrics"
)

var AllComponents = []Component{
	ComponentServer,
	ComponentExtAuthz,
	ComponentOpenFGA,
	ComponentValkey,
	ComponentSTS,
	ComponentPostgres,
	ComponentLogging,
	ComponentTelemetry,
	ComponentKafka,
	ComponentWorker,
	ComponentMetrics,
}

// Config represents the application configuration
type Config struct {
	MultitenancyEnabled bool                   `validate:"" mapstructure:"multitenancy_enabled"`
	Server              *ServerConfig          `validate:"" mapstructure:"server"`
	ExtAuthzService     *ExtAuthzServiceConfig `validate:"" mapstructure:"ext_authz_service"`
	OpenFGA             *OpenFGAConfig         `validate:"" mapstructure:"openfga"`
	Valkey              *ValkeyConfig          `validate:"" mapstructure:"valkey"`
	STS                 *STSConfig             `validate:"" mapstructure:"sts"`
	Postgres            *PostgresConfig        `validate:"" mapstructure:"postgres"`
	Logging             *LoggingConfig         `validate:"" mapstructure:"logging"`
	Telemetry           *TelemetryConfig       `validate:"" mapstructure:"telemetry"`
	Kafka               *KafkaConfig           `validate:"" mapstructure:"kafka"`
	Worker              *WorkerConfig          `validate:"" mapstructure:"worker"`
	Metrics             *MetricsConfig         `validate:"" mapstructure:"metrics"`
}

// ServerConfig contains server configuration
type ServerConfig struct {
	GRPCPort                int           `validate:"required,min=1,max=65535" mapstructure:"grpc_port"`
	HTTPPort                int           `validate:"required,min=1,max=65535" mapstructure:"http_port"`
	Host                    string        `validate:"required" mapstructure:"host"`
	GracefulShutdownTimeout time.Duration `validate:"" mapstructure:"shutdown_timeout"`
	Development             bool          `validate:"" mapstructure:"development"`
}

type ExtAuthzServiceConfig struct {
	JwkSetURL string `validate:"required,http_url" mapstructure:"jwk_set_url"`
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
	Address              string        `validate:"required" mapstructure:"address"`
	StoreID              string        `validate:"required" mapstructure:"store_id"`
	AuthorizationModelID string        `validate:"" mapstructure:"authorization_model_id"`
	ApiKey               string        `validate:"required" mapstructure:"api_key"`
	Timeout              time.Duration `validate:"" mapstructure:"timeout"`
}

// ValkeyConfig contains Valkey (Redis-compatible) configuration
type ValkeyConfig struct {
	Enabled  bool          `validate:"" mapstructure:"enabled"`
	Address  string        `validate:"required_if=Enabled true" mapstructure:"address"`
	Username string        `validate:"" mapstructure:"username"`
	Password string        `validate:"" mapstructure:"password"`
	DB       int           `validate:"min=0,max=15" mapstructure:"db"`
	PoolSize int           `validate:"min=1" mapstructure:"pool_size"`
	Timeout  time.Duration `validate:"" mapstructure:"timeout"`
	UseTLS   bool          `validate:"" mapstructure:"use_tls"`
}

// PostgresConfig contains PostgreSQL database configuration.
// Postgres 14 or later is required.
type PostgresConfig struct {
	Host            string        `validate:"required" mapstructure:"host"`
	Port            int           `validate:"required,min=1,max=65535" mapstructure:"port"`
	User            string        `validate:"required" mapstructure:"user"`
	Password        string        `validate:"" mapstructure:"password"`
	DBName          string        `validate:"required" mapstructure:"db_name"`
	SSLMode         string        `validate:"required,oneof=disable require verify-ca verify-full" mapstructure:"ssl_mode"`
	MaxOpenConns    int32         `validate:"min=1" mapstructure:"max_open_conns"`
	MaxIdleConns    int32         `validate:"min=1" mapstructure:"max_idle_conns"`
	ConnMaxLifetime time.Duration `validate:"" mapstructure:"conn_max_lifetime"`
	ConnMaxIdleTime time.Duration `validate:"" mapstructure:"conn_max_idle_time"`
	ConnectTimeout  time.Duration `validate:"" mapstructure:"connect_timeout"`
}

// DSN builds the PostgreSQL connection string (Data Source Name).
func (p *PostgresConfig) DSN() string {
	return fmt.Sprintf(
		"host=%s port=%d user=%s password=%s dbname=%s sslmode=%s connect_timeout=%d",
		p.Host,
		p.Port,
		p.User,
		p.Password,
		p.DBName,
		p.SSLMode,
		int(p.ConnectTimeout.Seconds()),
	)
}

// SetupLogger builds the application logger: a JSON or text handler on
// stdout, wrapped with trace/span correlation, tagged with static
// service/version attributes so every line can be attributed in Grafana.
func (c *LoggingConfig) SetupLogger(serviceName, serviceVersion string) *slog.Logger {
	var handler slog.Handler
	opts := &slog.HandlerOptions{
		Level:     ParseLogLevel(c.Level),
		AddSource: c.AddSource,
	}
	if c.Format == "json" {
		handler = slog.NewJSONHandler(os.Stdout, opts)
	} else {
		handler = slog.NewTextHandler(os.Stdout, opts)
	}
	return slog.New(logging.NewTraceHandler(handler)).With(
		"service", serviceName,
		"version", serviceVersion,
	)
}

// STSConfig contains Secure Token Service configuration
type STSConfig struct {
	Address              string        `validate:"required" mapstructure:"address"`
	UseTLS               bool          `validate:"" mapstructure:"use_tls"`
	Timeout              time.Duration `validate:"" mapstructure:"timeout"`
	EagerConnectionCheck bool          `validate:"" mapstructure:"eager_connection_check"`
}

// CreateSTSConnection creates a gRPC connection to the STS service
func (s *STSConfig) CreateSTSConnection() (*grpc.ClientConn, error) {
	return sts.CreateConnection(s.Address, s.UseTLS, s.Timeout, s.EagerConnectionCheck)
}

// KafkaConfig contains Kafka consumer configuration for the permission-update
// ingestion listener.
//
// FederatedServices is the registry source: each entry is a service slug, and
// the listener subscribes to the derived "permissions.<slug>" topic within a
// single consumer group. Topics are never hardcoded.
type KafkaConfig struct {
	Enabled                   bool     `validate:"" mapstructure:"enabled"`
	Brokers                   []string `validate:"required_if=Enabled true" mapstructure:"brokers"`
	FederatedServices         []string `validate:"" mapstructure:"federated_services"`
	FederatedServicesStrategy string   `validate:"omitempty,oneof=auto fs config" mapstructure:"federated_services_strategy"`
	ConsumerGroup             string   `validate:"required_if=Enabled true" mapstructure:"consumer_group"`
	TopicPartitions           int      `validate:"min=1" mapstructure:"topic_partitions"`
	TopicReplicationFactor    int      `validate:"min=1" mapstructure:"topic_replication_factor"`
}

// WorkerConfig contains configuration for the async permission-update worker
// that claims rows from permission_update_work and applies them to OpenFGA.
//
// The worker processes rows serially and scales horizontally by running multiple
// instances (FOR UPDATE SKIP LOCKED assigns disjoint rows), mirroring how the
// listener scales.
type WorkerConfig struct {
	Enabled        bool          `validate:"" mapstructure:"enabled"`
	BatchSize      int           `validate:"min=1" mapstructure:"batch_size"`
	PollInterval   time.Duration `validate:"" mapstructure:"poll_interval"`
	MaxAttempts    int           `validate:"min=1" mapstructure:"max_attempts"`
	RetryBackoff   time.Duration `validate:"" mapstructure:"retry_backoff"`
	StaleTimeout   time.Duration `validate:"" mapstructure:"stale_timeout"`
	ReaperInterval time.Duration `validate:"" mapstructure:"reaper_interval"`
}

// MetricsConfig contains configuration for the Prometheus metrics HTTP
// endpoint exposed by each long-running binary (serve, worker, listen) on its
// own dedicated port, as expected by the Istio mesh's Prometheus scrape config.
type MetricsConfig struct {
	Enabled bool   `validate:"" mapstructure:"enabled"`
	Port    int    `validate:"required,min=1,max=65535" mapstructure:"port"`
	Path    string `validate:"required" mapstructure:"path"`
}

// GetAddress returns the full metrics server address for the given host.
func (m *MetricsConfig) GetAddress(host string) string {
	return fmt.Sprintf("%s:%d", host, m.Port)
}

// LoggingConfig contains logging configuration
type LoggingConfig struct {
	Level     string `validate:"required,oneof=debug info warn error" mapstructure:"level"`
	Format    string `validate:"required,oneof=json text" mapstructure:"format"`
	AddSource bool   `validate:"" mapstructure:"add_source"`
}

// TelemetryConfig contains telemetry configuration
type TelemetryConfig struct {
	Enabled        bool   `validate:"" mapstructure:"enabled"`
	OTLPEndpoint   string `validate:"" mapstructure:"otlp_endpoint"`
	ServiceName    string `validate:"required_if=Enabled true" mapstructure:"service_name"`
	ServiceVersion string `validate:"required_if=Enabled true" mapstructure:"service_version"`
}

func (t *TelemetryConfig) SetupTelemetry(ctx context.Context, logger *slog.Logger) (trace.Tracer, func(context.Context) error, error) {
	if !t.Enabled {
		logger.InfoContext(ctx, "Telemetry is disabled")
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

var validate = validator.New()

func validateSection[T any](name string, ptr *T) error {
	if ptr == nil {
		return fmt.Errorf("%s configuration is missing", name)
	}
	if err := validate.Struct(ptr); err != nil {
		return fmt.Errorf("%s configuration validation failed: %w", name, err)
	}
	return nil
}

var componentValidators = map[Component]func(c *Config) error{
	ComponentServer: func(c *Config) error {
		return validateSection("server", c.Server)
	},
	ComponentExtAuthz: func(c *Config) error {
		return validateSection("ext_authz_service", c.ExtAuthzService)
	},
	ComponentOpenFGA: func(c *Config) error {
		if err := validateSection("openfga", c.OpenFGA); err != nil {
			return err
		}
		if c.OpenFGA.AuthorizationModelID == "" {
			return fmt.Errorf("openfga configuration validation failed: authorization_model_id is required")
		}
		return nil
	},
	ComponentOpenFGAModelWriter: func(c *Config) error {
		return validateSection("openfga", c.OpenFGA)
	},
	ComponentValkey: func(c *Config) error {
		return validateSection("valkey", c.Valkey)
	},
	ComponentSTS: func(c *Config) error {
		return validateSection("sts", c.STS)
	},
	ComponentPostgres: func(c *Config) error {
		return validateSection("postgres", c.Postgres)
	},
	ComponentLogging: func(c *Config) error {
		return validateSection("logging", c.Logging)
	},
	ComponentTelemetry: func(c *Config) error {
		return validateSection("telemetry", c.Telemetry)
	},
	ComponentKafka: func(c *Config) error {
		if err := validateSection("kafka", c.Kafka); err != nil {
			return err
		}
		if c.Kafka != nil && c.Kafka.Enabled && c.Kafka.FederatedServicesStrategy == "config" && len(c.Kafka.FederatedServices) == 0 {
			return fmt.Errorf("kafka configuration validation failed: federated_services is required when strategy is 'config'")
		}
		return nil
	},
	ComponentWorker: func(c *Config) error {
		return validateSection("worker", c.Worker)
	},
	ComponentMetrics: func(c *Config) error {
		return validateSection("metrics", c.Metrics)
	},
}

func (c *Config) ValidateComponents(components ...Component) error {
	for _, comp := range components {
		validatorFn, ok := componentValidators[comp]
		if !ok {
			return fmt.Errorf("unknown component: %s", comp)
		}
		if err := validatorFn(c); err != nil {
			return err
		}
	}

	return nil
}

func (c *Config) Validate() error {
	return c.ValidateComponents(AllComponents...)
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
