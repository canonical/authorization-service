package cmd

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/spf13/cobra"

	"github.com/canonical/authorization-service/config"
	"github.com/canonical/authorization-service/internal/metrics"
	"github.com/canonical/authorization-service/internal/repository"
	"github.com/canonical/authorization-service/internal/service/listen"
	"github.com/canonical/authorization-service/internal/version"
)

// kafkaStatsPollInterval is how often kafka-go's per-connection reader stats
// (offset, lag, queue length, deltas) are sampled and exported.
const kafkaStatsPollInterval = 15 * time.Second

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
	cfg, err := config.LoadConfigFor(cmd,
		config.ComponentPostgres,
		config.ComponentKafka,
		config.ComponentLogging,
		config.ComponentTelemetry,
		config.ComponentMetrics,
	)
	if err != nil {
		return err
	}

	logger := cfg.Logging.SetupLogger(cfg.Telemetry.ServiceName, version.Version)
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

	if err := integrations.InitKafkaConsumer(cfg, logger); err != nil {
		return fmt.Errorf("kafka consumer initialization failed: %w", err)
	}

	reg := metrics.NewRegistry()

	workRepo := repository.NewPostgresPermissionWorkRepository(integrations.Postgres, metrics.NewRepositoryRecorder(reg))
	ingestor := listen.NewIngestionService(
		integrations.ServiceRegistry,
		listen.NewDecoder(),
		listen.NewValidator(),
		workRepo,
		metrics.NewIngestRecorder(reg),
		logger,
	)

	listener := listen.NewListener(
		integrations.KafkaConsumer,
		ingestor,
		logger,
	)

	kafkaStatsRecorder := metrics.NewKafkaStatsRecorder(reg)

	runMetrics := cfg.Metrics.Enabled
	var metricsServer *http.Server
	if runMetrics {
		metricsServer = metrics.NewServer(cfg.Metrics.GetAddress(cfg.Server.Host), cfg.Metrics.Path, reg)
	}

	ctx, cancel := context.WithCancel(cmd.Context())
	defer cancel()

	numComponents := 2 // listener + kafka stats poller
	if runMetrics {
		numComponents++
	}

	type componentResult struct {
		name string
		err  error
	}

	errChan := make(chan componentResult, numComponents)
	go func() {
		errChan <- componentResult{name: "listener", err: listener.Run(ctx)}
	}()
	go func() {
		errChan <- componentResult{name: "kafka_stats", err: metrics.PollKafkaStats(ctx, integrations.KafkaConsumer, kafkaStatsRecorder, kafkaStatsPollInterval)}
	}()
	if runMetrics {
		go func() {
			logger.Info("Starting metrics server", "address", metricsServer.Addr)
			err := metricsServer.ListenAndServe()
			if errors.Is(err, http.ErrServerClosed) {
				err = nil
			} else if err != nil {
				logger.Error("Metrics server failed", "error", err)
			}
			errChan <- componentResult{name: "metrics", err: err}
		}()
	}

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	select {
	case res := <-errChan:
		logger.Info("Component stopped", "component", res.name)
		if res.err != nil {
			cancel()
			return fmt.Errorf("%s component failed: %w", res.name, res.err)
		}
		cancel()
	case sig := <-sigChan:
		logger.Info("Received signal, shutting down", "signal", sig)
		cancel()
	}

	if metricsServer != nil {
		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), cfg.Server.GracefulShutdownTimeout)
		if err := metricsServer.Shutdown(shutdownCtx); err != nil {
			logger.Error("Error during metrics server shutdown", "error", err)
		}
		shutdownCancel()
	}

	var firstErr error
	for i := 0; i < numComponents; i++ {
		res := <-errChan
		logger.Info("Component stopped", "component", res.name)
		if res.err != nil && firstErr == nil {
			firstErr = fmt.Errorf("%s component failed: %w", res.name, res.err)
		}
	}
	if firstErr != nil {
		return firstErr
	}

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), cfg.Server.GracefulShutdownTimeout)
	defer shutdownCancel()

	if err := tracerShutdown(shutdownCtx); err != nil {
		return fmt.Errorf("error shutting down tracer: %w", err)
	}

	logger.Info("Listener stopped")
	return nil
}
