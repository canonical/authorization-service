package cmd

import (
    "fmt"
    "io"
    "log/slog"

    "go.opentelemetry.io/otel/trace"
    "google.golang.org/grpc"

    stsv1 "github.com/canonical/authorization-service/client/v1/sts"
    "github.com/canonical/authorization-service/config"
    "github.com/canonical/authorization-service/internal/integration/nats"
    "github.com/canonical/authorization-service/internal/integration/openfga"
    "github.com/canonical/authorization-service/internal/integration/postgres"
    "github.com/canonical/authorization-service/internal/integration/sts"
    "github.com/canonical/authorization-service/internal/integration/valkey"
    "github.com/canonical/authorization-service/internal/repository"
    "github.com/canonical/authorization-service/internal/rule"
    "github.com/canonical/authorization-service/internal/service/authz"
    "github.com/canonical/authorization-service/internal/service/permissions"
)

type ClosableClientConnInterface interface {
    io.Closer
    grpc.ClientConnInterface
}

// Services holds all business logic services
type Services struct {
    Permissions   *permissions.Service
    Authz         *authz.Service
    ExternalAuthz *authz.ExternalAuthzService
}

// Integrations holds all external service clients
type Integrations struct {
    OpenFGA    openfga.ClientInterface
    OpenFGASDK openfga.OpenFGAClientInterface
    NATS       nats.EventClientInterface
    Valkey     valkey.CacheClientInterface
    STS        stsv1.SecurityTokenServiceClient
    Postgres   postgres.DBClientInterface
    stsConn    ClosableClientConnInterface
}

func (i *Integrations) initializeServices(tracer trace.Tracer, serviceLogger *slog.Logger) *Services {
    ruleRepo := repository.NewPostgresRuleRepository(i.Postgres)
    matcher := rule.NewRuleMatcher()
    resolver := rule.NewTupleResolver()
    resourceMapper := rule.NewResourceMapper(ruleRepo, matcher, resolver)

    return &Services{
        Permissions:   permissions.NewService(i.Valkey, i.NATS, serviceLogger),
        Authz:         authz.NewService(i.OpenFGA, serviceLogger),
        ExternalAuthz: authz.NewExternalAuthzService(i.STS, resourceMapper, i.OpenFGASDK, serviceLogger, tracer),
    }
}

func initializeIntegrations(cfg *config.Config, logger *slog.Logger, tracer trace.Tracer) (*Integrations, error) {
    var err error
    integrations := &Integrations{}

    // Initialize OpenFGA
    if cfg.OpenFGA.Enabled {
        integrations.OpenFGA, err = openfga.NewOpenFGAClient(
            cfg.OpenFGA.Address,
            cfg.OpenFGA.StoreID,
            cfg.OpenFGA.AuthKey,
            cfg.OpenFGA.UseTLS,
            logger,
        )

        if err != nil {
            return nil, fmt.Errorf("failed to create OpenFGA client: %w", err)
        }

    } else {
        logger.Info("OpenFGA disabled, using no-op client")
        integrations.OpenFGA = openfga.NewNoopClient(logger)
    }

    // Initialize NATS
    if cfg.NATS.Enabled {
        integrations.NATS, err = nats.NewClient(
            nats.Config{
                URL:             cfg.NATS.URL,
                ClusterID:       cfg.NATS.ClusterID,
                ClientID:        cfg.NATS.ClientID,
                EnableJetStream: cfg.NATS.EnableJetStream,
                StreamName:      cfg.NATS.StreamName,
                MaxReconnects:   cfg.NATS.MaxReconnects,
                ReconnectWait:   cfg.NATS.ReconnectWait,
                Timeout:         cfg.NATS.Timeout,
            },
            logger,
        )

        if err != nil {
            return nil, fmt.Errorf("failed to create NATS client: %w", err)
        }

    } else {
        logger.Info("NATS disabled, using no-op client")
        integrations.NATS = nats.NewNoopClient(logger)
    }

    // Initialize Valkey
    if cfg.Valkey.Enabled {
        integrations.Valkey, err = valkey.NewClient(
            valkey.Config{
                Address:  cfg.Valkey.Address,
                Password: cfg.Valkey.Password,
                DB:       cfg.Valkey.DB,
                PoolSize: cfg.Valkey.PoolSize,
                Timeout:  cfg.Valkey.Timeout,
                UseTLS:   cfg.Valkey.UseTLS,
            },
            logger,
        )

        if err != nil {
            return nil, fmt.Errorf("failed to create Valkey client: %w", err)
        }

    } else {
        logger.Info("Valkey disabled, using no-op client")
        integrations.Valkey = valkey.NewNoopClient(logger)
    }

    // Initialize Postgres
    if cfg.Postgres.Enabled {
        integrations.Postgres, err = postgres.NewClient(
            postgres.Config{
                Host:            cfg.Postgres.Host,
                Port:            cfg.Postgres.Port,
                User:            cfg.Postgres.User,
                Password:        cfg.Postgres.Password,
                DBName:          cfg.Postgres.DBName,
                SSLMode:         cfg.Postgres.SSLMode,
                MaxOpenConns:    cfg.Postgres.MaxOpenConns,
                MaxIdleConns:    cfg.Postgres.MaxIdleConns,
                ConnMaxLifetime: cfg.Postgres.ConnMaxLifetime,
                ConnMaxIdleTime: cfg.Postgres.ConnMaxIdleTime,
                ConnectTimeout:  cfg.Postgres.ConnectTimeout,
            },
            logger,
            tracer,
        )

        if err != nil {
            return nil, fmt.Errorf("failed to create Postgres client: %w", err)
        }

    } else {
        logger.Info("Postgres disabled, using no-op client")
        integrations.Postgres = postgres.NewNoopClient(logger)
    }

    // Initialize STS
    if cfg.STS.Enabled {
        integrations.stsConn, err = cfg.STS.CreateSTSConnection()
        if err != nil {
            return nil, fmt.Errorf("failed to create STS connection: %w", err)
        }

        integrations.STS = sts.NewSTSClientWrapper(
            stsv1.NewSecurityTokenServiceClient(integrations.stsConn),
            logger,
            tracer,
        )

    } else {
        logger.Info("STS disabled, using no-op client")
        integrations.STS = sts.NewNoopClient(logger)
    }

    return integrations, nil
}

func (i *Integrations) cleanupIntegrations(logger *slog.Logger) {

    if i.OpenFGA != nil {
        if err := i.OpenFGA.Close(); err != nil {
            logger.Error("Failed to close OpenFGA client", "error", err)
        }
    }

    if i.NATS != nil {
        if err := i.NATS.Close(); err != nil {
            logger.Error("Failed to close NATS client", "error", err)
        }
    }

    if i.Valkey != nil {
        if err := i.Valkey.Close(); err != nil {
            logger.Error("Failed to close Valkey client", "error", err)
        }
    }

    if i.Postgres != nil {
        i.Postgres.Close()
    }

    if i.stsConn != nil {
        if err := i.stsConn.Close(); err != nil {
            logger.Error("Failed to close STS connection", "error", err)
        }
    }
}
