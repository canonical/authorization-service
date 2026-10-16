// Copyright 2026 Canonical Ltd.
// SPDX-License-Identifier: AGPL-3.0-only

package cmd

import (
	"fmt"

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
		config.ComponentServer,
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
	logger.InfoContext(cmd.Context(), "Starting permission-update worker", "version", version.Version)

	tracer, tracerShutdown, err := cfg.Telemetry.SetupTelemetry(cmd.Context(), logger)
	if err != nil {
		return fmt.Errorf("telemetry setup failed: %w", err)
	}

	integrations, err := config.InitializeIntegrations(
		cfg,
		logger,
		tracer,
		config.ComponentOpenFGA,
		config.ComponentPostgres,
	)
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

	runners := []ComponentRunner{
		{Name: "worker", Run: w.Run},
	}
	if !noReaper {
		r := worker.NewReaper(
			workRepo,
			cfg.Worker.StaleTimeout,
			cfg.Worker.ReaperInterval,
			metrics.NewReaperRecorder(reg),
			logger,
		)
		runners = append(runners, ComponentRunner{Name: "reaper", Run: r.Run})
	}

	if err := runManagedProcess(cmd.Context(), cfg, logger, reg, tracerShutdown, runners...); err != nil {
		return err
	}

	logger.InfoContext(cmd.Context(), "Worker stopped")
	return nil
}

var noReaper bool

func init() {
	workerCmd.Flags().BoolVar(&noReaper, "no-reaper", false, "Disable the in-process stale-row reaper")
}
