// Copyright 2026 Canonical Ltd.
// SPDX-License-Identifier: AGPL-3.0-only

package cmd

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/canonical/authorization-service/config"
	kafkaintegration "github.com/canonical/authorization-service/internal/integration/kafka"
	"github.com/canonical/authorization-service/internal/service/listen"
	"github.com/canonical/authorization-service/internal/version"
)

var ensureTopicsCmd = &cobra.Command{
	Use:   "ensure-topics",
	Short: "Idempotently create the permission-update topics for federated services",
	Long: `Create the "permissions.<slug>" Kafka topic for every federated service
listed in FEDERATED_SERVICES. Existing topics are left untouched, so this command
is safe to run repeatedly (e.g. as part of a federation sync step).
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

	registry, err := listen.NewServiceRegistry(cfg.Kafka.FederatedServices)
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

	if err := kafkaintegration.EnsureTopics(cmd.Context(), cfg.Kafka.Brokers, specs, logger); err != nil {
		return fmt.Errorf("failed to ensure topics: %w", err)
	}

	return nil
}

func init() {
	rootCmd.AddCommand(ensureTopicsCmd)
}
