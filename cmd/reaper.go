// Copyright 2026 Canonical Ltd.
// SPDX-License-Identifier: AGPL-3.0-only

package cmd

import (
	"context"
	"fmt"

	"github.com/kelseyhightower/envconfig"
	"github.com/spf13/cobra"

	"github.com/canonical/authorization-service/config"
	"github.com/canonical/authorization-service/internal/repository"
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
	cfg := &config.Config{}
	if err := envconfig.Process("", cfg); err != nil {
		return fmt.Errorf("failed to load configuration: %w", err)
	}

	if err := cfg.Validate(); err != nil {
		return fmt.Errorf("invalid configuration: %w", err)
	}

	logger := cfg.Logging.SetupLogger()
	logger.Info("Starting one-off stale-row reaper run")

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

	logger.Info("Reclaiming stale rows...", "stale_timeout", cfg.Worker.StaleTimeout.String())
	count, err := workRepo.ReclaimStale(cmd.Context(), cfg.Worker.StaleTimeout)
	if err != nil {
		logger.Error("Failed to reclaim stale rows", "error", err)
		return err
	}

	logger.Info("One-off stale-row reaper completed successfully", "reclaimed_count", count)

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), cfg.Server.GracefulShutdownTimeout)
	defer shutdownCancel()

	if err := tracerShutdown(shutdownCtx); err != nil {
		return fmt.Errorf("error shutting down tracer: %w", err)
	}

	return nil
}
