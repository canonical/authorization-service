package cmd

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/kelseyhightower/envconfig"
	"github.com/spf13/cobra"

	"github.com/canonical/authorization-service/config"
	"github.com/canonical/authorization-service/internal/repository"
	"github.com/canonical/authorization-service/internal/service/listen"
	"github.com/canonical/authorization-service/internal/version"
)

var listenCmd = &cobra.Command{
	Use:   "listen",
	Short: "Start the Kafka permission-update listener",
	Long: `Start the Kafka listener that consumes permission-update events from the
federated services' "<slug>.permissions" topics and durably persists them into
the PostgreSQL work table for downstream tuple application.
Configuration is loaded from environment variables.`,
	RunE: runListen,
}

func runListen(cmd *cobra.Command, _ []string) error {
	cfg := &config.Config{}
	if err := envconfig.Process("", cfg); err != nil {
		return fmt.Errorf("failed to load configuration: %w", err)
	}

	if err := cfg.Validate(); err != nil {
		return fmt.Errorf("invalid configuration: %w", err)
	}

	logger := cfg.Logging.SetupLogger()
	logger.Info("Starting Kafka listener", "version", version.Version)

	tracer, tracerShutdown, err := cfg.Telemetry.SetupTelemetry(cmd.Context(), logger)
	if err != nil {
		return fmt.Errorf("telemetry setup failed: %w", err)
	}

	integrations, err := config.InitializeIntegrations(cfg, logger, tracer)
	if err != nil {
		return fmt.Errorf("integration initialization failed: %w", err)
	}
	defer integrations.CleanupIntegrations(logger)

	workRepo := repository.NewPostgresPermissionWorkRepository(integrations.Postgres)
	ingestor := listen.NewIngestionService(
		integrations.ServiceRegistry,
		listen.NewDecoder(),
		listen.NewValidator(),
		workRepo,
		nil, // metrics: no-op until an OTel-backed implementation is wired
		logger,
	)

	listener := listen.NewListener(
		integrations.KafkaConsumer,
		ingestor,
		logger,
	)

	ctx, cancel := context.WithCancel(cmd.Context())
	defer cancel()

	errChan := make(chan error, 1)
	go func() {
		errChan <- listener.Run(ctx)
	}()

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	select {
	case err := <-errChan:
		logger.Error("Listener error", "error", err)
		return err
	case sig := <-sigChan:
		logger.Info("Received signal, shutting down", "signal", sig)
		cancel()
	}

	if err := <-errChan; err != nil {
		logger.Error("Error during listener shutdown", "error", err)
		return err
	}

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), cfg.Server.GracefulShutdownTimeout)
	defer shutdownCancel()

	if err := tracerShutdown(shutdownCtx); err != nil {
		return fmt.Errorf("error shutting down tracer: %w", err)
	}

	logger.Info("Listener stopped")
	return nil
}
