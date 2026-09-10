// Copyright 2026 Canonical Ltd.
// SPDX-License-Identifier: AGPL-3.0-only

package config

import (
	"context"
	"fmt"
	"io"
	"log/slog"

	"github.com/coreos/go-oidc/v3/oidc"
	"github.com/openfga/go-sdk/client"
	"github.com/openfga/go-sdk/credentials"
	"github.com/prometheus/client_golang/prometheus"
	"go.opentelemetry.io/otel/trace"
	"google.golang.org/grpc"

	stsv1 "github.com/canonical/authorization-service/client/v1/sts"
	kafkaintegration "github.com/canonical/authorization-service/internal/integration/kafka"
	"github.com/canonical/authorization-service/internal/integration/openfga"
	"github.com/canonical/authorization-service/internal/integration/postgres"
	"github.com/canonical/authorization-service/internal/integration/sts"
	"github.com/canonical/authorization-service/internal/integration/valkey"
	"github.com/canonical/authorization-service/internal/metrics"
	ruleRepository "github.com/canonical/authorization-service/internal/repository"
	"github.com/canonical/authorization-service/internal/service/authz"
	"github.com/canonical/authorization-service/internal/service/listen"
	"github.com/canonical/authorization-service/internal/service/permissions"
	"github.com/canonical/authorization-service/internal/service/rules"
)

type ClosableClientConnInterface interface {
	io.Closer
	grpc.ClientConnInterface
}

// Services holds all business logic services
type Services struct {
	Permissions   *permissions.Service
	ExternalAuthz *authz.ExternalAuthzService
}

// Integrations holds all external service clients
type Integrations struct {
	jwkSetUrl           string
	MultitenancyEnabled bool
	OpenFGA             openfga.OpenFGAClientInterface
	Valkey              valkey.CacheClientInterface
	STS                 stsv1.SecurityTokenServiceClient
	Postgres            postgres.DBClientInterface
	KafkaConsumer       kafkaintegration.ConsumerInterface
	ServiceRegistry     *listen.ServiceRegistry

	stsConn ClosableClientConnInterface
}

func InitializeIntegrations(cfg *Config, logger *slog.Logger, tracer trace.Tracer) (*Integrations, error) {
	var err error
	integrations := &Integrations{
		jwkSetUrl:           cfg.ExtAuthzService.JwkSetURL,
		MultitenancyEnabled: cfg.MultitenancyEnabled,
	}

	// Initialize OpenFGA
	if cfg.OpenFGA.AuthorizationModelID == "" {
		return nil, fmt.Errorf("authorization_model_id is required")
	}

	creds, err := credentials.NewCredentials(credentials.Credentials{
		Method: credentials.CredentialsMethodApiToken,
		Config: &credentials.Config{
			ApiToken: cfg.OpenFGA.ApiKey,
		},
	})
	if err != nil {
		return nil, fmt.Errorf("error loading OpenFGA auth credentials: %v", err)
	}

	openfgaClient, err := client.NewSdkClient(
		&client.ClientConfiguration{
			ApiUrl:               cfg.OpenFGA.Address,
			Credentials:          creds,
			AuthorizationModelId: cfg.OpenFGA.AuthorizationModelID,
			Telemetry:            nil,
		},
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create OpenFGA SDK client: %w", err)
	}

	if err := openfgaClient.SetStoreId(cfg.OpenFGA.StoreID); err != nil {
		return nil, fmt.Errorf("failed to set OpenFGA store ID: %w", err)
	}

	integrations.OpenFGA = openfgaClient

	// Initialize Valkey
	if cfg.Valkey.Enabled {
		integrations.Valkey, err = valkey.NewClient(
			valkey.Config{
				Address:  cfg.Valkey.Address,
				Username: cfg.Valkey.Username,
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

	// Initialize Kafka
	if cfg.Kafka.Enabled {
		registry, err := listen.NewServiceRegistry(cfg.Kafka.FederatedServices)
		if err != nil {
			return nil, fmt.Errorf("failed to build federated service registry: %w", err)
		}
		integrations.ServiceRegistry = registry

		kafkaClient, err := kafkaintegration.NewClient(
			kafkaintegration.Config{
				Brokers:       cfg.Kafka.Brokers,
				ConsumerGroup: cfg.Kafka.ConsumerGroup,
				Topics:        registry.Topics(),
			},
			logger,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to create Kafka client: %w", err)
		}
		integrations.KafkaConsumer = kafkaClient
	} else {
		logger.Info("Kafka disabled, using no-op client")
		integrations.KafkaConsumer = kafkaintegration.NewNoopClient(logger)
	}

	// Initialize STS
	integrations.stsConn, err = cfg.STS.CreateSTSConnection()
	if err != nil {
		return nil, fmt.Errorf("failed to create STS connection: %w", err)
	}

	integrations.STS = sts.NewSTSClientWrapper(
		stsv1.NewSecurityTokenServiceClient(integrations.stsConn),
		logger,
		tracer,
	)

	return integrations, nil
}

// InitializeServices builds the business-logic services. reg registers the
// recorders backing each service's Metrics seam; pass a fresh
// internal/metrics.NewRegistry() per process.
func (i *Integrations) InitializeServices(tracer trace.Tracer, serviceLogger *slog.Logger, reg *prometheus.Registry) *Services {
	ruleRepo := ruleRepository.NewPostgresRuleRepository(i.Postgres, metrics.NewRepositoryRecorder(reg))
	matcher := rules.NewRuleMatcher()
	resolver := rules.NewTupleResolver()
	resourceMapper := rules.NewResourceMapper(ruleRepo, matcher, resolver)

	keySet := oidc.NewRemoteKeySet(context.Background(), i.jwkSetUrl)
	verifier := oidc.NewVerifier("", keySet, &oidc.Config{
		SkipClientIDCheck:    true,
		SkipIssuerCheck:      true,
		SupportedSigningAlgs: []string{oidc.RS256, oidc.ES256},
	})

	return &Services{
		Permissions:   permissions.NewService(i.Valkey, metrics.NewPermissionsRecorder(reg), serviceLogger),
		ExternalAuthz: authz.NewExternalAuthzService(verifier, i.STS, resourceMapper, i.OpenFGA, i.MultitenancyEnabled, metrics.NewCheckRecorder(reg), serviceLogger, tracer),
	}
}

func (i *Integrations) CleanupIntegrations(logger *slog.Logger) {

	if i.KafkaConsumer != nil {
		if err := i.KafkaConsumer.Close(); err != nil {
			logger.Error("Failed to close Kafka consumer", "error", err)
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
