// Copyright 2026 Canonical Ltd.
// SPDX-License-Identifier: AGPL-3.0-only

package cmd

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"github.com/spf13/cobra"

	"github.com/canonical/authorization-service/config"
	"github.com/canonical/authorization-service/internal/metrics"
	"github.com/canonical/authorization-service/internal/repository"
	"github.com/canonical/authorization-service/internal/service/listen"
	"github.com/canonical/authorization-service/internal/service/worker"
	"github.com/canonical/authorization-service/internal/version"
)

var workerCmd = &cobra.Command{
	Use:   "worker",
	Short: "Start the async permission-update worker",
	Long: `Start the worker that claims permission-update rows from the PostgreSQL
work table and applies the corresponding tuple changes to OpenFGA, idempotently.
Failed applications are retried up to a configurable limit before being marked
failed. Run multiple instances to scale horizontally.
Configuration is loaded from environment variables.`,
	RunE: runWorker,
}

func runWorker(cmd *cobra.Command, _ []string) error {
	cfg, err := config.LoadConfigFor(cmd,
		config.ComponentWorker,
		config.ComponentOpenFGA,
		config.ComponentPostgres,
		config.ComponentLogging,
		config.ComponentTelemetry,
		config.ComponentMetrics,
	)
	if err != nil {
		return err
	}

	logger := cfg.Logging.SetupLogger(cfg.Telemetry.ServiceName, version.Version)
	logger.Info("Starting permission-update worker", "version", version.Version)

	tracer, tracerShutdown, err := cfg.Telemetry.SetupTelemetry(cmd.Context(), logger)
	if err != nil {
		return fmt.Errorf("telemetry setup failed: %w", err)
	}

	integrations, err := config.InitializeIntegrations(cfg, logger, tracer)
	if err != nil {
		return fmt.Errorf("integration initialization failed: %w", err)
	}
	defer integrations.CleanupIntegrations(logger)

	reg := metrics.NewRegistry()

	workRepo := repository.NewPostgresPermissionWorkRepository(integrations.Postgres, metrics.NewRepositoryRecorder(reg))
	applier := worker.NewOpenFGAApplier(integrations.OpenFGA, cfg.OpenFGA.AuthorizationModelID)
	processor := worker.NewProcessor(
		workRepo,
		applier,
		listen.NewDecoder(),
		cfg.Worker.MaxAttempts,
		cfg.MultitenancyEnabled,
		metrics.NewProcessorRecorder(reg),
		logger,
	)
	w := worker.NewWorker(
		workRepo,
		processor,
		cfg.Worker.BatchSize,
		cfg.Worker.PollInterval,
		cfg.Worker.RetryBackoff,
		metrics.NewWorkerLoopRecorder(reg),
		logger,
	)

	runReaper := !noReaper
	var r *worker.Reaper
	if runReaper {
		r = worker.NewReaper(
			workRepo,
			cfg.Worker.StaleTimeout,
			cfg.Worker.ReaperInterval,
			metrics.NewReaperRecorder(reg),
			logger,
		)
	}

	runMetrics := cfg.Metrics.Enabled
	var metricsServer *http.Server
	if runMetrics {
		metricsServer = metrics.NewServer(cfg.Metrics.GetAddress(cfg.Server.Host), cfg.Metrics.Path, reg)
	}

	ctx, cancel := context.WithCancel(cmd.Context())
	defer cancel()

	numComponents := 1
	if runReaper {
		numComponents++
	}
	if runMetrics {
		numComponents++
	}

	type componentResult struct {
		name string
		err  error
	}

	errChan := make(chan componentResult, numComponents)
	go func() {
		errChan <- componentResult{name: "worker", err: w.Run(ctx)}
	}()
	if runReaper {
		go func() {
			errChan <- componentResult{name: "reaper", err: r.Run(ctx)}
		}()
	}
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

	// Wait for all started components to exit
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

	logger.Info("Worker stopped")
	return nil
}

var noReaper bool

func init() {
	workerCmd.Flags().BoolVar(&noReaper, "no-reaper", false, "Disable the in-process stale-row reaper")
}
