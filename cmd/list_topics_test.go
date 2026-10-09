// Copyright 2026 Canonical Ltd.
// SPDX-License-Identifier: AGPL-3.0-only

package cmd

import (
	"bytes"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTopicsListCmd_AutoStrategy(t *testing.T) {
	buf := new(bytes.Buffer)
	listTopicsCmd.SetOut(buf)
	t.Cleanup(func() { listTopicsCmd.SetOut(nil) })

	err := runListTopics(listTopicsCmd, nil)
	require.NoError(t, err)

	output := strings.TrimSpace(buf.String())
	lines := strings.Split(output, "\n")

	assert.NotEmpty(t, lines)
	for _, line := range lines {
		assert.True(t, strings.HasPrefix(line, "permissions."), "expected topic to start with 'permissions.', got %q", line)
	}
}

func TestTopicsListCmd_ExplicitConfigStrategy(t *testing.T) {
	t.Setenv("FEDERATED_SERVICES_STRATEGY", "config")
	t.Setenv("FEDERATED_SERVICES", "service-a,service-b")

	buf := new(bytes.Buffer)
	listTopicsCmd.SetOut(buf)
	t.Cleanup(func() { listTopicsCmd.SetOut(nil) })

	err := runListTopics(listTopicsCmd, nil)
	require.NoError(t, err)

	output := strings.TrimSpace(buf.String())
	lines := strings.Split(output, "\n")

	expected := []string{"permissions.service-a", "permissions.service-b"}
	assert.Equal(t, expected, lines)
}

func TestTopicsListCmd_SubcommandExecution(t *testing.T) {
	buf := new(bytes.Buffer)
	rootCmd.SetOut(buf)
	listTopicsCmd.SetOut(buf)
	t.Cleanup(func() {
		rootCmd.SetOut(nil)
		listTopicsCmd.SetOut(nil)
	})

	rootCmd.SetArgs([]string{"topics", "list"})

	err := rootCmd.Execute()
	require.NoError(t, err)

	output := strings.TrimSpace(buf.String())
	lines := strings.Split(output, "\n")

	assert.NotEmpty(t, lines)
	for _, line := range lines {
		assert.True(t, strings.HasPrefix(line, "permissions."), "expected topic to start with 'permissions.', got %q", line)
	}
}

func TestTopicsListCmd_TopLevelAliasExecution(t *testing.T) {
	buf := new(bytes.Buffer)
	rootCmd.SetOut(buf)
	listTopicsTopLevelCmd.SetOut(buf)
	t.Cleanup(func() {
		rootCmd.SetOut(nil)
		listTopicsTopLevelCmd.SetOut(nil)
	})

	rootCmd.SetArgs([]string{"list-topics"})

	err := rootCmd.Execute()
	require.NoError(t, err)

	output := strings.TrimSpace(buf.String())
	lines := strings.Split(output, "\n")

	assert.NotEmpty(t, lines)
	for _, line := range lines {
		assert.True(t, strings.HasPrefix(line, "permissions."), "expected topic to start with 'permissions.', got %q", line)
	}
}
