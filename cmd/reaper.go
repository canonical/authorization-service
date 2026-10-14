// Copyright 2026 Canonical Ltd.
// SPDX-License-Identifier: AGPL-3.0-only

package cmd

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/canonical/authorization-service/config"
	"github.com/canonical/authorization-service/internal/repository"
	"github.com/canonical/authorization-service/internal/version"
)

var reaperCmd = &cobra.Command{
	Use:   "reaper",
	Short: "Run a one-off stale-row reclamation",
	Long: `Run a one-off scan of the permission_update_work table to reclaim rows
stuck in the 'processing' state for longer than the configured timeout (WORKER_STALE_TIMEOUT)
and return them to 'received'.
This command is suitable for manual execution or external scheduling (e.g. cron).`,
	RunE: runReaperCmd,
}

func runReaperCmd(cmd *cobra.Command, _ []string) error {
	cfg, err := config.LoadConfigFor(cmd,
		config.ComponentServer,
		config.ComponentPostgres,
		config.ComponentWorker,
		config.ComponentLogging,
		config.ComponentTelemetry,
	)
	if err != nil {
		return err
	}

	logger := cfg.Logging.SetupLogger(cfg.Telemetry.ServiceName, version.Version)
	logger.InfoContext(cmd.Context(), "Starting one-off stale-row reaper run")

	tracer, tracerShutdown, err := cfg.Telemetry.SetupTelemetry(cmd.Context(), logger)
	if err != nil {
		return fmt.Errorf("telemetry setup failed: %w", err)
	}

	integrations, err := config.InitializeIntegrations(cfg, logger, tracer, config.ComponentPostgres)
	if err != nil {
		return fmt.Errorf("integration initialization failed: %w", err)
	}
	defer integrations.CleanupIntegrations(logger)

	// This is a one-off batch job, not continuously scraped, so no metrics recorder is wired.
	workRepo := repository.NewPostgresPermissionWorkRepository(integrations.Postgres, nil)

	runner := ComponentRunner{
		Name: "reaper",
		Run: func(ctx context.Context) error {
			logger.InfoContext(ctx, "Reclaiming stale rows...", "stale_timeout", cfg.Worker.StaleTimeout.String())
			count, err := workRepo.ReclaimStale(ctx, cfg.Worker.StaleTimeout)
			if err != nil {
				logger.ErrorContext(ctx, "Failed to reclaim stale rows", "error", err)
				return err
			}
			logger.InfoContext(ctx, "One-off stale-row reaper completed successfully", "reclaimed_count", count)
			return nil
		},
	}

	return runManagedProcess(cmd.Context(), cfg, logger, nil, tracerShutdown, runner)
}
