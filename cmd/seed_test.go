// Copyright 2026 Canonical Ltd.
// SPDX-License-Identifier: AGPL-3.0-only

package cmd

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupTestRequiredEnv(t *testing.T) {
	t.Setenv("SERVER_DEVELOPMENT", "true")
	t.Setenv("OPEN_FGA_STORE_ID", "test-store")
	t.Setenv("OPEN_FGA_AUTHORIZATION_MODEL_ID", "test-model")
	t.Setenv("OPEN_FGA_API_KEY", "test-key")
}

func newTestSeedCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:  "seed",
		RunE: runSeedCmd,
	}
	cmd.Flags().String("config", "", "")
	cmd.Flags().StringP("dir", "d", "authz/model/services", "Directory to scan for rules.yaml files")
	cmd.Flags().Bool("dry-run", false, "Only validate route rules files")
	return cmd
}

func TestSeedCmd_DirWithoutDryRunErrors(t *testing.T) {
	setupTestRequiredEnv(t)

	cmd := newTestSeedCmd()

	// GIVEN --dir is specified but --dry-run is false (default)
	err := cmd.Flags().Set("dir", "some/physical/dir")
	require.NoError(t, err)

	// WHEN executing the command
	err = cmd.Execute()

	// THEN it should fail with a specific flag constraint error
	assert.ErrorContains(t, err, "the --dir / -d flag can only be used during dry-run validation")
}

func TestSeedCmd_DryRunEmbeddedSuccess(t *testing.T) {
	setupTestRequiredEnv(t)

	cmd := newTestSeedCmd()

	// GIVEN --dry-run is true and no custom --dir is specified
	err := cmd.Flags().Set("dry-run", "true")
	require.NoError(t, err)

	// WHEN executing the command
	err = cmd.Execute()

	// THEN it should validate embedded rules successfully and return nil
	assert.NoError(t, err)
}

func TestSeedCmd_DryRunPhysicalDirSuccess(t *testing.T) {
	setupTestRequiredEnv(t)

	// GIVEN a temporary directory with a valid rules.yaml
	tempDir := t.TempDir()
	rulesContent := `version: "1"
service: "temp-service"
revision: "1.0.0"
description: "Temp rules"
rules:
  - method: "GET"
    match: "/api/v1/temp/{id}"
    priority: 100
    tuples:
      - userResourceType: "user"
        permission: "viewer"
        objectResourceType: "temp"
        objectResourceId: "{id}"
`
	err := os.WriteFile(filepath.Join(tempDir, "rules.yaml"), []byte(rulesContent), 0644)
	require.NoError(t, err)

	cmd := newTestSeedCmd()

	// AND --dry-run is true and --dir points to our tempDir
	err = cmd.Flags().Set("dry-run", "true")
	require.NoError(t, err)
	err = cmd.Flags().Set("dir", tempDir)
	require.NoError(t, err)

	// WHEN executing the command
	err = cmd.Execute()

	// THEN validation succeeds
	assert.NoError(t, err)
}

func TestSeedCmd_DryRunPhysicalDirFailure(t *testing.T) {
	setupTestRequiredEnv(t)

	// GIVEN a temporary directory with an invalid rules.yaml (invalid method, empty service)
	tempDir := t.TempDir()
	rulesContent := `version: "1"
service: ""
revision: "1.0.0"
rules:
  - method: "INVALID_METHOD"
    match: "/api"
    tuples:
      - userResourceType: "u"
        permission: "p"
        objectResourceType: "o"
        objectResourceId: "id"
`
	err := os.WriteFile(filepath.Join(tempDir, "rules.yaml"), []byte(rulesContent), 0644)
	require.NoError(t, err)

	cmd := newTestSeedCmd()

	// AND --dry-run is true and --dir points to our tempDir
	err = cmd.Flags().Set("dry-run", "true")
	require.NoError(t, err)
	err = cmd.Flags().Set("dir", tempDir)
	require.NoError(t, err)

	// WHEN executing the command
	err = cmd.Execute()

	// THEN validation should fail
	assert.Error(t, err)
	assert.ErrorContains(t, err, "validation failed for 1 file(s)")
}
