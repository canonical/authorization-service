// Copyright 2026 Canonical Ltd.
// SPDX-License-Identifier: AGPL-3.0-only

package cmd

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/spf13/cobra"

	"github.com/canonical/authorization-service/config"
	"github.com/canonical/authorization-service/internal/health"
	"github.com/canonical/authorization-service/internal/metrics"
	"github.com/canonical/authorization-service/internal/repository"
	"github.com/canonical/authorization-service/internal/service/listen"
	"github.com/canonical/authorization-service/internal/version"
	"github.com/canonical/authorization-service/server/grpc"
)

// kafkaStatsPollInterval is how often kafka-go's per-connection reader stats
// (offset, lag, queue length, deltas) are sampled and exported.
const kafkaStatsPollInterval = 15 * time.Second

var listenCmd = &cobra.Command{
	Use:   "listen",
	Short: "Start the permission-update listener",
	Long: `Start the permission-update listener in platform (Kafka), standalone (gRPC),
or hybrid (Kafka + gRPC) mode to validate and durably persist permission-update
envelopes into the PostgreSQL work table for downstream tuple application.
Configuration is loaded from environment variables.`,
	RunE: runListen,
}

func runListen(cmd *cobra.Command, _ []string) error {
	cfg, err := config.LoadConfigFor(cmd,
		config.ComponentServer,
		config.ComponentPostgres,
		config.ComponentLogging,
		config.ComponentTelemetry,
		config.ComponentMetrics,
		config.ComponentListener,
	)
	if err != nil {
		return err
	}

	mode := cfg.Listener.GetMode()
	runKafka := mode == config.ListenModePlatform || mode == config.ListenModeHybrid
	runGRPC := mode == config.ListenModeStandalone || mode == config.ListenModeHybrid

	logger := cfg.Logging.SetupLogger(cfg.Telemetry.ServiceName, version.Version)
	logger.Info("Starting permission-update listener", "version", version.Version, "mode", mode)

	// Automatically ensure topics exist before starting Kafka listener unless --no-topics is set.
	if runKafka && !noTopics {
		if err := ensureTopics(cmd.Context(), cfg, logger); err != nil {
			return fmt.Errorf("ensuring topics failed: %w", err)
		}
	}
	tracer, tracerShutdown, err := cfg.Telemetry.SetupTelemetry(cmd.Context(), logger)
	if err != nil {
		return fmt.Errorf("telemetry setup failed: %w", err)
	}

	integrations, err := config.InitializeIntegrations(cfg, logger, tracer)
	if err != nil {
		return fmt.Errorf("integration initialization failed: %w", err)
	}
	defer integrations.CleanupIntegrations(logger)

	if err := integrations.InitServiceRegistry(cfg); err != nil {
		return fmt.Errorf("service registry initialization failed: %w", err)
	}

	if runKafka {
		if err := integrations.InitKafkaConsumer(cfg, logger); err != nil {
			return fmt.Errorf("kafka consumer initialization failed: %w", err)
		}
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

	ctx, cancel := context.WithCancel(cmd.Context())
	defer cancel()

	var (
		listener   *listen.Listener
		grpcServer *grpc.Server
	)

	numComponents := 1 // health server
	if runKafka {
		listener = listen.NewListener(
			integrations.KafkaConsumer,
			ingestor,
			logger,
		)
		numComponents += 2 // listener + kafka stats poller
	}
	if runGRPC {
		verifier := config.NewOIDCVerifier(ctx, cfg.ExtAuthzService.JwkSetURL)
		grpcIngestServer := listen.NewGRPCIngestionServer(verifier, ingestor, logger)
		grpcServer, err = grpc.NewServer(
			cfg.Server,
			logger,
			reg,
			grpc.WithPermissionIngestionService(grpcIngestServer),
		)
		if err != nil {
			return fmt.Errorf("gRPC server creation failed: %w", err)
		}
		numComponents++
	}

	healthServer := health.NewServer(cfg.Server.GetHTTPAddress())

	runMetrics := cfg.Metrics.Enabled
	var metricsServer *http.Server
	if runMetrics {
		metricsServer = metrics.NewServer(cfg.Metrics.GetAddress(cfg.Server.Host), cfg.Metrics.Path, reg)
		numComponents++
	}

	type componentResult struct {
		name string
		err  error
	}

	errChan := make(chan componentResult, numComponents)
	if runKafka {
		statsRecorder := metrics.NewKafkaStatsRecorder(reg)
		go func() {
			errChan <- componentResult{name: "listener", err: listener.Run(ctx)}
		}()
		go func() {
			errChan <- componentResult{name: "kafka_stats", err: metrics.PollKafkaStats(ctx, integrations.KafkaConsumer, statsRecorder, kafkaStatsPollInterval)}
		}()
	}
	if runGRPC {
		go func() {
			logger.Info("Starting gRPC ingestion server", "address", grpcServer.Addr())
			err := grpcServer.Start()
			if errors.Is(err, net.ErrClosed) {
				err = nil
			} else if err != nil {
				logger.Error("gRPC ingestion server failed", "error", err)
			}
			errChan <- componentResult{name: "grpc", err: err}
		}()
	}
	go func() {
		logger.Info("Starting health server", "address", healthServer.Addr)
		err := healthServer.ListenAndServe()
		if errors.Is(err, http.ErrServerClosed) {
			err = nil
		} else if err != nil {
			logger.Error("Health server failed", "error", err)
		}
		errChan <- componentResult{name: "health", err: err}
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

	var firstErr error
	remaining := numComponents

	select {
	case res := <-errChan:
		remaining--
		logger.Info("Component stopped", "component", res.name)
		if res.err != nil {
			firstErr = fmt.Errorf("%s component failed: %w", res.name, res.err)
		}
		cancel()
	case sig := <-sigChan:
		logger.Info("Received signal, shutting down", "signal", sig)
		cancel()
	}

	if grpcServer != nil {
		grpcServer.GracefulStop()
	}

	healthShutdownCtx, healthShutdownCancel := context.WithTimeout(context.Background(), cfg.Server.GracefulShutdownTimeout)
	if err := healthServer.Shutdown(healthShutdownCtx); err != nil {
		logger.Error("Error during health server shutdown", "error", err)
	}
	healthShutdownCancel()

	if metricsServer != nil {
		metricsShutdownCtx, metricsShutdownCancel := context.WithTimeout(context.Background(), cfg.Server.GracefulShutdownTimeout)
		if err := metricsServer.Shutdown(metricsShutdownCtx); err != nil {
			logger.Error("Error during metrics server shutdown", "error", err)
		}
		metricsShutdownCancel()
	}

	for i := 0; i < remaining; i++ {
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

var noTopics bool

func init() {
	listenCmd.Flags().BoolVar(&noTopics, "no-topics", false, "Disable automatic topic creation before listening")
	listenCmd.Flags().String("mode", config.ListenModePlatform, "Listener operating mode (platform, standalone, hybrid)")
}
