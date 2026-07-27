// Copyright 2026 Canonical Ltd.
// SPDX-License-Identifier: AGPL-3.0-only

package cmd

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/spf13/cobra"

	"github.com/canonical/authorization-service/config"
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
	cfg, err := config.LoadConfig(cmd)
	if err != nil {
		return err
	}

	logger := cfg.Logging.SetupLogger()
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

	workRepo := repository.NewPostgresPermissionWorkRepository(integrations.Postgres)
	applier := worker.NewOpenFGAApplier(integrations.OpenFGA, cfg.OpenFGA.AuthorizationModelID)
	processor := worker.NewProcessor(
		workRepo,
		applier,
		listen.NewDecoder(),
		cfg.Worker.MaxAttempts,
		nil, // metrics: no-op until an OTel-backed implementation is wired
		logger,
	)
	w := worker.NewWorker(
		workRepo,
		processor,
		cfg.Worker.BatchSize,
		cfg.Worker.PollInterval,
		cfg.Worker.RetryBackoff,
		logger,
	)

	runReaper := !noReaper
	var r *worker.Reaper
	if runReaper {
		r = worker.NewReaper(
			workRepo,
			cfg.Worker.StaleTimeout,
			cfg.Worker.ReaperInterval,
			logger,
		)
	}

	ctx, cancel := context.WithCancel(cmd.Context())
	defer cancel()

	numComponents := 1
	if runReaper {
		numComponents = 2
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

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	select {
	case res := <-errChan:
		if res.err != nil {
			logger.Error("Component error", "component", res.name, "error", res.err)
			cancel()
			return fmt.Errorf("%s component failed: %w", res.name, res.err)
		}
		cancel()
	case sig := <-sigChan:
		logger.Info("Received signal, shutting down", "signal", sig)
		cancel()
	}

	// Wait for all started components to exit
	var firstErr error
	for i := 0; i < numComponents; i++ {
		if res := <-errChan; res.err != nil {
			logger.Error("Error during component shutdown", "component", res.name, "error", res.err)
			if firstErr == nil {
				firstErr = fmt.Errorf("%s component failed: %w", res.name, res.err)
			}
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
