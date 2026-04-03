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
    Server          *ServerConfig          `validate:"required"`
    ExtAuthzService *ExtAuthzServiceConfig `validate:"required"`
    OpenFGA         *OpenFGAConfig         `validate:"required"`
    NATS            *NATSConfig            `validate:"required"`
    Valkey          *ValkeyConfig          `validate:"required"`
    STS             *STSConfig             `validate:"required"`
    Postgres        *PostgresConfig        `validate:"required"`
    Logging         *LoggingConfig         `validate:"required"`
    Telemetry       *TelemetryConfig       `validate:"required"`
}

// ServerConfig contains server configuration
type ServerConfig struct {
    GRPCPort                int           `validate:"required,min=1,max=65535" env:"GRPC_PORT" default:"9090"`
    HTTPPort                int           `validate:"required,min=1,max=65535" env:"HTTP_PORT" default:"8080"`
    Host                    string        `validate:"required" env:"SERVER_HOST" default:"0.0.0.0"`
    GracefulShutdownTimeout time.Duration `validate:"" env:"SERVER_SHUTDOWN_TIMEOUT" default:"15s"`
    Development             bool          `validate:"required" env:"DEV" default:"false"`
}

type ExtAuthzServiceConfig struct {
    JwkSetURL string `validate:"required,http_url" env:"EXTAUTHZ_JWK_SET_URL" default:"http://localhost:9091/.well-known/jwks.json"`
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
    Enabled bool          `validate:"" env:"OPENFGA_ENABLED" default:"false"`
    Address string        `validate:"required_if=Enabled true" env:"OPENFGA_ADDRESS" default:"localhost:8081"`
    StoreID string        `validate:"" env:"OPENFGA_STORE_ID"`
    AuthKey string        `validate:"" env:"OPENFGA_AUTH_KEY"`
    UseTLS  bool          `validate:"" env:"OPENFGA_USE_TLS" default:"false"`
    Timeout time.Duration `validate:"" env:"OPENFGA_TIMEOUT" default:"10s"`
}

// NATSConfig contains NATS configuration
type NATSConfig struct {
    Enabled         bool          `validate:"" env:"NATS_ENABLED" default:"false"`
    URL             string        `validate:"required" env:"NATS_URL" default:"nats://localhost:4222"`
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
    Enabled  bool          `validate:"" env:"VALKEY_ENABLED" default:"false"`
    Address  string        `validate:"required" env:"VALKEY_ADDRESS" default:"localhost:6379"`
    Password string        `validate:"" env:"VALKEY_PASSWORD"`
    DB       int           `validate:"min=0,max=15" env:"VALKEY_DB" default:"0"`
    PoolSize int           `validate:"min=1" env:"VALKEY_POOL_SIZE" default:"10"`
    Timeout  time.Duration `validate:"" env:"VALKEY_TIMEOUT" default:"5s"`
    UseTLS   bool          `validate:"" env:"VALKEY_USE_TLS" default:"false"`
}

// PostgresConfig contains PostgreSQL database configuration.
// Postgres 14 or later is required.
type PostgresConfig struct {
    Enabled         bool          `validate:"" env:"POSTGRES_ENABLED" default:"false"`
    Host            string        `validate:"required" env:"POSTGRES_HOST" default:"localhost"`
    Port            int           `validate:"required,min=1,max=65535" env:"POSTGRES_PORT" default:"5432"`
    User            string        `validate:"required_if=Enabled true" env:"POSTGRES_USER" default:"postgres"`
    Password        string        `validate:"" env:"POSTGRES_PASSWORD"`
    DBName          string        `validate:"required_if=Enabled true" env:"POSTGRES_DB" default:"authz"`
    SSLMode         string        `validate:"required,oneof=disable require verify-ca verify-full" env:"POSTGRES_SSL_MODE" default:"disable"`
    MaxOpenConns    int32         `validate:"min=1" env:"POSTGRES_MAX_OPEN_CONNS" default:"25"`
    MaxIdleConns    int32         `validate:"min=1" env:"POSTGRES_MAX_IDLE_CONNS" default:"5"`
    ConnMaxLifetime time.Duration `validate:"" env:"POSTGRES_CONN_MAX_LIFETIME" default:"30m"`
    ConnMaxIdleTime time.Duration `validate:"" env:"POSTGRES_CONN_MAX_IDLE_TIME" default:"5m"`
    ConnectTimeout  time.Duration `validate:"" env:"POSTGRES_CONNECT_TIMEOUT" default:"10s"`
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
    Enabled              bool          `validate:"" env:"STS_ENABLED" default:"false"`
    Address              string        `validate:"required" env:"STS_ADDRESS" default:"localhost:9091"`
    UseTLS               bool          `validate:"" env:"STS_USE_TLS" default:"false"`
    Timeout              time.Duration `validate:"" env:"STS_TIMEOUT" default:"10s"`
    EagerConnectionCheck bool          `validate:"" env:"STS_EAGER_CONNECTION_CHECK" default:"false"`
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
