package cmd

import (
	"fmt"
	"log/slog"

	"github.com/canonical/authorization-service/internal/config"
	"github.com/canonical/authorization-service/internal/integrations/nats"
	"github.com/canonical/authorization-service/internal/integrations/openfga"
	"github.com/canonical/authorization-service/internal/integrations/sts"
	"github.com/canonical/authorization-service/internal/integrations/valkey"
	"github.com/canonical/authorization-service/internal/service/authz"
	"github.com/canonical/authorization-service/internal/service/permissions"
)

// Services holds all business logic services
type Services struct {
	Permissions *permissions.Service
	Authz       *authz.Service
}

// Integrations holds all external service clients
type Integrations struct {
	OpenFGA openfga.Client
	NATS    *nats.Client
	Valkey  *valkey.Client
	STS     *sts.Client

	logger *slog.Logger
}

func (i *Integrations) initializeServices(serviceLogger *slog.Logger) *Services {
	return &Services{
		Permissions: permissions.NewService(i.Valkey, i.NATS, serviceLogger),
		Authz:       authz.NewService(i.OpenFGA, i.STS, i.Valkey, serviceLogger),
	}
}

func initializeIntegrations(cfg *config.Config, logger *slog.Logger) (*Integrations, error) {
	var err error = nil
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

	// Initialize Valkey
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

	// Initialize STS
	integrations.STS, err = sts.NewClient(
		sts.Config{
			Address: cfg.STS.Address,
			UseTLS:  cfg.STS.UseTLS,
			Timeout: cfg.STS.Timeout,
		},
		logger,
	)

	if err != nil {
		return nil, fmt.Errorf("failed to create STS client: %w", err)
	}

	return integrations, nil
}

func (i *Integrations) cleanupIntegrations() {

	if i.OpenFGA != nil {
		if err := i.OpenFGA.Close(); err != nil {
			i.logger.Error("Failed to close OpenFGA client", "error", err)
		}
	}

	if i.NATS != nil {
		if err := i.NATS.Close(); err != nil {
			i.logger.Error("Failed to close NATS client", "error", err)
		}
	}

	if i.Valkey != nil {
		if err := i.Valkey.Close(); err != nil {
			i.logger.Error("Failed to close Valkey client", "error", err)
		}
	}

	if i.STS != nil {
		if err := i.STS.Close(); err != nil {
			i.logger.Error("Failed to close STS client", "error", err)
		}
	}
}
