// Copyright 2026 Canonical Ltd.
// SPDX-License-Identifier: AGPL-3.0-only

package cmd

import (
	"github.com/spf13/cobra"
)

var topicsCmd = &cobra.Command{
	Use:   "topics",
	Short: "Kafka topics management commands",
	Long: `Kafka topics management commands.
This command group provides utilities for managing and listing Kafka topics for federated services.`,
}

// Top-level aliases for backwards compatibility
var ensureTopicsTopLevelCmd = &cobra.Command{
	Use:    "ensure-topics",
	Short:  "Idempotently create permission-update topics (alias for 'topics ensure')",
	Hidden: true,
	RunE:   runEnsureTopics,
}

var listTopicsTopLevelCmd = &cobra.Command{
	Use:    "list-topics",
	Short:  "List Kafka topic names (alias for 'topics list')",
	Hidden: true,
	RunE:   runListTopics,
}

func init() {
	topicsCmd.AddCommand(ensureTopicsCmd)
	topicsCmd.AddCommand(listTopicsCmd)

	rootCmd.AddCommand(topicsCmd)
	rootCmd.AddCommand(ensureTopicsTopLevelCmd)
	rootCmd.AddCommand(listTopicsTopLevelCmd)
}
