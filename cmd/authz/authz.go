// Copyright 2025 Canonical Ltd.
// SPDX-License-Identifier: AGPL-3.0

package authz

import (
    "github.com/spf13/cobra"
)

// Command represents the authz command group
var Command = &cobra.Command{
    Use:   "authz",
    Short: "OpenFGA authorization model management commands",
    Long: `OpenFGA authorization model management commands.
This command group provides utilities for managing authorization models
in the OpenFGA authorization system.`,
}

func init() {
    Command.AddCommand(WriteModelCmd)
}
