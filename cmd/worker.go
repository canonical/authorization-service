// Copyright 2026 Canonical Ltd.
// SPDX-License-Identifier: AGPL-3.0-only

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
	cfg := &config.Config{}
	if err := envconfig.Process("", cfg); err != nil {
		return fmt.Errorf("failed to load configuration: %w", err)
	}

	if err := cfg.Validate(); err != nil {
		return fmt.Errorf("invalid configuration: %w", err)
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

	ctx, cancel := context.WithCancel(cmd.Context())
	defer cancel()

	errChan := make(chan error, 1)
	go func() {
		errChan <- w.Run(ctx)
	}()

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	select {
	case err := <-errChan:
		logger.Error("Worker error", "error", err)
		return err
	case sig := <-sigChan:
		logger.Info("Received signal, shutting down", "signal", sig)
		cancel()
	}

	if err := <-errChan; err != nil {
		logger.Error("Error during worker shutdown", "error", err)
		return err
	}

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), cfg.Server.GracefulShutdownTimeout)
	defer shutdownCancel()

	if err := tracerShutdown(shutdownCtx); err != nil {
		return fmt.Errorf("error shutting down tracer: %w", err)
	}

	logger.Info("Worker stopped")
	return nil
}
