// Copyright 2026 Canonical Ltd.
// SPDX-License-Identifier: AGPL-3.0-only

package cmd

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/canonical/authorization-service/config"
)

var listTopicsCmd = &cobra.Command{
	Use:     "list",
	Aliases: []string{"list-topics"},
	Short:   "List Kafka topic names obtained from the service registry",
	Long: `List the "permissions.<slug>" Kafka topic names for all federated services
discovered or configured according to the FEDERATED_SERVICES_STRATEGY.`,
	RunE: runListTopics,
}

func runListTopics(cmd *cobra.Command, _ []string) error {
	cfg, err := config.LoadConfigFor(cmd,
		config.ComponentKafka,
	)
	if err != nil {
		return err
	}

	registry, err := config.BuildServiceRegistry(cfg.Kafka)
	if err != nil {
		return fmt.Errorf("failed to build federated service registry: %w", err)
	}

	out := cmd.OutOrStdout()
	for _, topic := range registry.Topics() {
		fmt.Fprintln(out, topic)
	}

	return nil
}
