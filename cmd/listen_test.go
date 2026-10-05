// Copyright 2026 Canonical Ltd.
// SPDX-License-Identifier: AGPL-3.0-only

package cmd

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestListenCmd_NoTopicsFlag(t *testing.T) {
	// Reset flag state for testing to verify default behaviour
	noTopics = false

	flag := listenCmd.Flags().Lookup("no-topics")
	require.NotNil(t, flag, "expected --no-topics flag to be registered")
	assert.Equal(t, "false", flag.DefValue, "expected default value of --no-topics flag to be false")

	// Verify setting flag updates noTopics variable
	err := listenCmd.Flags().Set("no-topics", "true")
	require.NoError(t, err)
	assert.True(t, noTopics, "expected noTopics to be true after setting --no-topics flag")

	// Reset flag state after test execution
	_ = listenCmd.Flags().Set("no-topics", "false")
}
