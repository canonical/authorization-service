// Copyright 2026 Canonical Ltd.
// SPDX-License-Identifier: AGPL-3.0-only

package cmd

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWorkerCmd_NoReaperFlag(t *testing.T) {
	noReaper = false

	flag := workerCmd.Flags().Lookup("no-reaper")
	require.NotNil(t, flag, "expected --no-reaper flag to be registered")
	assert.Equal(t, "false", flag.DefValue, "expected default value of --no-reaper flag to be false")

	err := workerCmd.Flags().Set("no-reaper", "true")
	require.NoError(t, err)
	assert.True(t, noReaper, "expected noReaper to be true after setting --no-reaper flag")

	_ = workerCmd.Flags().Set("no-reaper", "false")
}

func TestRunWorker_InvalidHTTPPortFailsValidation(t *testing.T) {
	t.Setenv("OPENFGA_STORE_ID", "test-store")
	t.Setenv("OPENFGA_AUTHORIZATION_MODEL_ID", "test-model")
	t.Setenv("OPENFGA_API_KEY", "test-key")
	t.Setenv("HTTP_PORT", "70000")

	err := runWorker(workerCmd, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "server configuration validation failed")
}
