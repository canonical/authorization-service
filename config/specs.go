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
	MultitenancyEnabled bool                   `validate:"" envconfig:"MULTITENANCY_ENABLED" mapstructure:"multitenancy_enabled" default:"false"`
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
	GRPCPort                int           `validate:"required,min=1,max=65535" envconfig:"GRPC_PORT" mapstructure:"grpc_port" default:"9091"`
	HTTPPort                int           `validate:"required,min=1,max=65535" envconfig:"HTTP_PORT" mapstructure:"http_port" default:"8070"`
	Host                    string        `validate:"required" envconfig:"SERVER_HOST" mapstructure:"host" default:"0.0.0.0"`
	GracefulShutdownTimeout time.Duration `validate:"" envconfig:"SERVER_SHUTDOWN_TIMEOUT" mapstructure:"shutdown_timeout" default:"15s"`
	Development             bool          `validate:"" envconfig:"DEV" mapstructure:"development" default:"false"`
}

type ExtAuthzServiceConfig struct {
	JwkSetURL      string `validate:"required,http_url" envconfig:"EXTAUTHZ_JWK_SET_URL" mapstructure:"jwk_set_url" default:"http://localhost:8080/.well-known/jwks.json"`
	HydraJwkSetURL string `validate:"required,required_with=HydraIssuer,http_url" envconfig:"EXTAUTHZ_HYDRA_JWK_SET_URL" mapstructure:"hydra_jwk_set_url" default:"http://localhost:4444/.well-known/jwks.json"`
	HydraIssuer    string `validate:"required,required_with=HydraJwkSetURL,http_url" envconfig:"EXTAUTHZ_HYDRA_ISSUER" mapstructure:"hydra_issuer" default:"http://localhost:4444/"`
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
	AuthorizationModelID string        `validate:"" envconfig:"OPENFGA_AUTHORIZATION_MODEL_ID" mapstructure:"authorization_model_id"`
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
	DBName          string        `validate:"required" envconfig:"POSTGRES_DB" mapstructure:"db_name" default:"authorization-service"`
	SSLMode         string        `validate:"required,oneof=disable require verify-ca verify-full" envconfig:"POSTGRES_SSL_MODE" mapstructure:"ssl_mode" default:"disable"`
	MaxOpenConns    int32         `validate:"min=1" envconfig:"POSTGRES_MAX_OPEN_CONNS" mapstructure:"max_open_conns" default:"25"`
	MaxIdleConns    int32         `validate:"min=1" envconfig:"POSTGRES_MAX_IDLE_CONNS" mapstructure:"max_idle_conns" default:"5"`
	ConnMaxLifetime time.Duration `validate:"" envconfig:"POSTGRES_CONN_MAX_LIFETIME" mapstructure:"conn_max_lifetime" default:"30m"`
	ConnMaxIdleTime time.Duration `validate:"" envconfig:"POSTGRES_CONN_MAX_IDLE_TIME" mapstructure:"conn_max_idle_time" default:"5m"`
	ConnectTimeout  time.Duration `validate:"" envconfig:"POSTGRES_CONNECT_TIMEOUT" mapstructure:"connect_timeout" default:"10s"`
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
// the listener subscribes to the derived "permissions.<slug>" topic within a
// single consumer group. Topics are never hardcoded.
type KafkaConfig struct {
	Enabled                   bool     `validate:"" envconfig:"KAFKA_ENABLED" mapstructure:"enabled" default:"false"`
	Brokers                   []string `validate:"required_if=Enabled true" envconfig:"KAFKA_BROKERS" mapstructure:"brokers" default:"localhost:9092"`
	FederatedServices         []string `validate:"" envconfig:"FEDERATED_SERVICES" mapstructure:"federated_services"`
	FederatedServicesStrategy string   `validate:"omitempty,oneof=auto fs config" envconfig:"FEDERATED_SERVICES_STRATEGY" mapstructure:"federated_services_strategy" default:"auto"`
	ConsumerGroup             string   `validate:"required_if=Enabled true" envconfig:"KAFKA_CONSUMER_GROUP" mapstructure:"consumer_group" default:"authz-listener"`
	TopicPartitions           int      `validate:"min=1" envconfig:"KAFKA_TOPIC_PARTITIONS" mapstructure:"topic_partitions" default:"1"`
	TopicReplicationFactor    int      `validate:"min=1" envconfig:"KAFKA_TOPIC_REPLICATION_FACTOR" mapstructure:"topic_replication_factor" default:"1"`
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

// MetricsConfig contains configuration for the Prometheus metrics HTTP
// endpoint exposed by each long-running binary (serve, worker, listen) on its
// own dedicated port, as expected by the Istio mesh's Prometheus scrape config.
type MetricsConfig struct {
	Enabled bool   `validate:"" envconfig:"METRICS_ENABLED" mapstructure:"enabled" default:"true"`
	Port    int    `validate:"required,min=1,max=65535" envconfig:"METRICS_PORT" mapstructure:"port" default:"9100"`
	Path    string `validate:"required" envconfig:"METRICS_PATH" mapstructure:"path" default:"/metrics"`
}

// GetAddress returns the full metrics server address for the given host.
func (m *MetricsConfig) GetAddress(host string) string {
	return fmt.Sprintf("%s:%d", host, m.Port)
}

// LoggingConfig contains logging configuration
type LoggingConfig struct {
	Level     string `validate:"required,oneof=debug info warn error" envconfig:"LOG_LEVEL" mapstructure:"level" default:"info"`
	Format    string `validate:"required,oneof=json text" envconfig:"LOG_FORMAT" mapstructure:"format" default:"json"`
	AddSource bool   `validate:"" envconfig:"LOG_ADD_SOURCE" mapstructure:"add_source" default:"false"`
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
