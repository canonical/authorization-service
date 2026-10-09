// Copyright 2026 Canonical Ltd.
// SPDX-License-Identifier: AGPL-3.0-only

package cmd

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/spf13/cobra"

	"github.com/canonical/authorization-service/config"
	kafkaintegration "github.com/canonical/authorization-service/internal/integration/kafka"
	"github.com/canonical/authorization-service/internal/version"
)

var ensureTopicsCmd = &cobra.Command{
	Use:     "ensure",
	Aliases: []string{"ensure-topics"},
	Short:   "Idempotently create the permission-update topics for federated services",
    Long: `Create the "permissions.<slug>" Kafka topic for all federated services discovered or configured according to FEDERATED_SERVICES_STRATEGY. Existing topics are left untouched, so this command is safe to run repeatedly.
Configuration is loaded from environment variables.`,
	RunE: runEnsureTopics,
}

func runEnsureTopics(cmd *cobra.Command, _ []string) error {
	cfg, err := config.LoadConfigFor(cmd,
		config.ComponentKafka,
		config.ComponentLogging,
		config.ComponentTelemetry,
	)
	if err != nil {
		return err
	}

	logger := cfg.Logging.SetupLogger(cfg.Telemetry.ServiceName, version.Version)

	return ensureTopics(cmd.Context(), cfg, logger)
}

// ensureTopics idempotently creates the permission-update topics for federated services.
func ensureTopics(ctx context.Context, cfg *config.Config, logger *slog.Logger) error {
	registry, err := config.BuildServiceRegistry(cfg.Kafka)
	if err != nil {
		return fmt.Errorf("failed to build federated service registry: %w", err)
	}

	specs := make([]kafkaintegration.TopicSpec, 0, len(registry.Topics()))
	for _, topic := range registry.Topics() {
		specs = append(specs, kafkaintegration.TopicSpec{
			Name:              topic,
			NumPartitions:     cfg.Kafka.TopicPartitions,
			ReplicationFactor: cfg.Kafka.TopicReplicationFactor,
		})
	}

	if err := kafkaintegration.EnsureTopics(ctx, cfg.Kafka.Brokers, specs, logger); err != nil {
		return fmt.Errorf("failed to ensure topics: %w", err)
	}

	return nil
}
