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

func TestRunListen_InvalidHTTPPortFailsValidation(t *testing.T) {
	t.Setenv("HTTP_PORT", "0")

	err := runListen(listenCmd, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "server configuration validation failed")
}

func TestListenCmd_ModeFlag(t *testing.T) {
	flag := listenCmd.Flags().Lookup("mode")
	require.NotNil(t, flag, "expected --mode flag to be registered")
	assert.Equal(t, "platform", flag.DefValue, "expected default value of --mode flag to be platform")

	t.Cleanup(func() {
		_ = listenCmd.Flags().Set("mode", "platform")
		flag.Changed = false
	})

	require.NoError(t, listenCmd.Flags().Set("mode", "standalone"))
	assert.Equal(t, "standalone", flag.Value.String())
}

func TestRunListen_ValidationByMode(t *testing.T) {
	t.Run("standalone mode fails when EXTAUTHZ_JWK_SET_URL is invalid", func(t *testing.T) {
		t.Setenv("LISTENER_MODE", "standalone")
		t.Setenv("EXTAUTHZ_JWK_SET_URL", "not-a-valid-url")

		err := runListen(listenCmd, nil)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "ext_authz_service configuration validation failed")
	})

	t.Run("platform mode fails when KAFKA_BROKERS is empty", func(t *testing.T) {
		t.Setenv("LISTENER_MODE", "platform")
		t.Setenv("KAFKA_BROKERS", "   ")

		err := runListen(listenCmd, nil)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "brokers is required for platform mode")
	})

	t.Run("invalid mode fails listener validation", func(t *testing.T) {
		t.Setenv("LISTENER_MODE", "unsupported")

		err := runListen(listenCmd, nil)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "listener configuration validation failed")
	})
}
