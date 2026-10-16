// Copyright 2026 Canonical Ltd.
// SPDX-License-Identifier: AGPL-3.0-only

package cmd

import (
	"context"
	"fmt"
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
federated services' "permissions.<slug>" topics and durably persists them into
the PostgreSQL work table for downstream tuple application.
Configuration is loaded from environment variables.`,
	RunE: runListen,
}

func runListen(cmd *cobra.Command, _ []string) error {
	cfg, err := config.LoadConfigFor(cmd,
		config.ComponentServer,
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
	logger.InfoContext(cmd.Context(), "Starting Kafka listener", "version", version.Version)

	// Automatically ensure topics exist before starting listener unless --no-topics is set.
	if !noTopics {
		if err := ensureTopics(cmd.Context(), cfg, logger); err != nil {
			return fmt.Errorf("ensuring topics failed: %w", err)
		}
	}

	tracer, tracerShutdown, err := cfg.Telemetry.SetupTelemetry(cmd.Context(), logger)
	if err != nil {
		return fmt.Errorf("telemetry setup failed: %w", err)
	}

	integrations, err := config.InitializeIntegrations(
		cfg,
		logger,
		tracer,
		config.ComponentPostgres,
		config.ComponentKafka,
	)
	if err != nil {
		return fmt.Errorf("integration initialization failed: %w", err)
	}
	defer integrations.CleanupIntegrations(logger)

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

	runners := []ComponentRunner{
		{Name: "listener", Run: listener.Run},
		{
			Name: "kafka_stats",
			Run: func(ctx context.Context) error {
				return metrics.PollKafkaStats(ctx, integrations.KafkaConsumer, kafkaStatsRecorder, kafkaStatsPollInterval)
			},
		},
	}

	if err := runManagedProcess(cmd.Context(), cfg, logger, reg, tracerShutdown, runners...); err != nil {
		return err
	}

	logger.InfoContext(cmd.Context(), "Listener stopped")
	return nil
}

var noTopics bool

func init() {
	listenCmd.Flags().BoolVar(&noTopics, "no-topics", false, "Disable automatic topic creation before listening")
}
