package config

import (
	"fmt"
	"log/slog"
	"os"
	"time"

	"github.com/go-playground/validator/v10"
)

// Config represents the application configuration
type Config struct {
	Server    *ServerConfig    `validate:"required"`
	OpenFGA   *OpenFGAConfig   `validate:"required"`
	NATS      *NATSConfig      `validate:"required"`
	Valkey    *ValkeyConfig    `validate:"required"`
	STS       *STSConfig       `validate:"required"`
	Logging   *LoggingConfig   `validate:"required"`
	Telemetry *TelemetryConfig `validate:"required"`
}

// ServerConfig contains server configuration
type ServerConfig struct {
	GRPCPort                int           `validate:"required,min=1,max=65535" env:"GRPC_PORT" default:"9090"`
	HTTPPort                int           `validate:"required,min=1,max=65535" env:"HTTP_PORT" default:"8080"`
	Host                    string        `validate:"required" env:"SERVER_HOST" default:"0.0.0.0"`
	GracefulShutdownTimeout time.Duration `validate:"" env:"SERVER_SHUTDOWN_TIMEOUT" default:"15s"`
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
	Enabled bool          `validate:"" env:"OPENFGA_ENABLED" default:"true"`
	Address string        `validate:"required_if=Enabled true" env:"OPENFGA_ADDRESS" default:"localhost:8081"`
	StoreID string        `validate:"" env:"OPENFGA_STORE_ID"`
	AuthKey string        `validate:"" env:"OPENFGA_AUTH_KEY"`
	UseTLS  bool          `validate:"" env:"OPENFGA_USE_TLS" default:"false"`
	Timeout time.Duration `validate:"" env:"OPENFGA_TIMEOUT" default:"10s"`
}

// NATSConfig contains NATS configuration
type NATSConfig struct {
	Enabled         bool          `validate:"" env:"NATS_ENABLED" default:"true"`
	URL             string        `validate:"required_if=Enabled true" env:"NATS_URL" default:"nats://localhost:4222"`
	ClusterID       string        `validate:"" env:"NATS_CLUSTER_ID" default:"authz-cluster"`
	ClientID        string        `validate:"" env:"NATS_CLIENT_ID" default:"authz-service"`
	EnableJetStream bool          `validate:"" env:"NATS_ENABLE_JETSTREAM" default:"true"`
	StreamName      string        `validate:"required_if=EnableJetStream true" env:"NATS_STREAM_NAME" default:"AUTHZ"`
	MaxReconnects   int           `validate:"min=0" env:"NATS_MAX_RECONNECTS" default:"10"`
	ReconnectWait   time.Duration `validate:"" env:"NATS_RECONNECT_WAIT" default:"2s"`
	Timeout         time.Duration `validate:"" env:"NATS_TIMEOUT" default:"10s"`
}

// ValkeyConfig contains Valkey (Redis-compatible) configuration
type ValkeyConfig struct {
	Enabled  bool          `validate:"" env:"VALKEY_ENABLED" default:"true"`
	Address  string        `validate:"required_if=Enabled true" env:"VALKEY_ADDRESS" default:"localhost:6379"`
	Password string        `validate:"" env:"VALKEY_PASSWORD"`
	DB       int           `validate:"min=0,max=15" env:"VALKEY_DB" default:"0"`
	PoolSize int           `validate:"min=1" env:"VALKEY_POOL_SIZE" default:"10"`
	Timeout  time.Duration `validate:"" env:"VALKEY_TIMEOUT" default:"5s"`
	UseTLS   bool          `validate:"" env:"VALKEY_USE_TLS" default:"false"`
}

func (c *LoggingConfig) SetupLogger() *slog.Logger {
	var handler slog.Handler
	opts := &slog.HandlerOptions{
		Level: parseLogLevel(c.Level),
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
	Address string        `validate:"required" env:"STS_ADDRESS" default:"localhost:9091"`
	UseTLS  bool          `validate:"" env:"STS_USE_TLS" default:"false"`
	Timeout time.Duration `validate:"" env:"STS_TIMEOUT" default:"10s"`
}

// LoggingConfig contains logging configuration
type LoggingConfig struct {
	Level  string `validate:"required,oneof=debug info warn error" env:"LOG_LEVEL" default:"info"`
	Format string `validate:"required,oneof=json text" env:"LOG_FORMAT" default:"json"`
}

// TelemetryConfig contains telemetry configuration
type TelemetryConfig struct {
	Enabled        bool   `validate:"" env:"TELEMETRY_ENABLED" default:"false"`
	OTLPEndpoint   string `validate:"" env:"OTEL_EXPORTER_OTLP_ENDPOINT"`
	ServiceName    string `validate:"required" env:"OTEL_SERVICE_NAME" default:"authorization-service"`
	ServiceVersion string `validate:"required" env:"OTEL_SERVICE_VERSION" default:"v1.0.0"`
}

// Validate validates the configuration using the validator
func (c *Config) Validate() error {
	validate := validator.New()
	if err := validate.Struct(c); err != nil {
		return fmt.Errorf("configuration validation failed: %w", err)
	}
	return nil
}

func parseLogLevel(level string) slog.Level {
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
