// Copyright 2026 Canonical Ltd.
// SPDX-License-Identifier: AGPL-3.0-only

package cmd

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/canonical/authorization-service/config"
	"github.com/canonical/authorization-service/internal/metrics"
)

func discardCmdLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func TestRunManagedProcess_ContextCancellationAndMetricsShutdown(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	cfg := &config.Config{
		Server: &config.ServerConfig{
			Host:                    "127.0.0.1",
			GracefulShutdownTimeout: 2 * time.Second,
		},
		Metrics: &config.MetricsConfig{
			Enabled: true,
			Port:    0,
			Path:    "/metrics",
		},
	}

	reg := metrics.NewRegistry()
	var tracerClosed bool
	tracerShutdown := func(context.Context) error {
		tracerClosed = true
		return nil
	}

	started := make(chan struct{})
	runner := ComponentRunner{
		Name: "worker",
		Run: func(runCtx context.Context) error {
			close(started)
			<-runCtx.Done()
			return nil
		},
	}

	go func() {
		<-started
		time.Sleep(30 * time.Millisecond)
		cancel()
	}()

	err := runManagedProcess(ctx, cfg, discardCmdLogger(), reg, tracerShutdown, runner)
	require.NoError(t, err)
	assert.True(t, tracerClosed)
}

func TestRunManagedProcess_SignalTermination(t *testing.T) {
	cfg := &config.Config{
		Server: &config.ServerConfig{
			Host:                    "127.0.0.1",
			GracefulShutdownTimeout: 2 * time.Second,
		},
		Metrics: &config.MetricsConfig{
			Enabled: false,
		},
	}

	started := make(chan struct{})
	runner := ComponentRunner{
		Name: "listener",
		Run: func(runCtx context.Context) error {
			close(started)
			<-runCtx.Done()
			return nil
		},
	}

	go func() {
		<-started
		time.Sleep(20 * time.Millisecond)
		_ = syscall.Kill(syscall.Getpid(), syscall.SIGINT)
	}()

	err := runManagedProcess(context.Background(), cfg, discardCmdLogger(), nil, nil, runner)
	require.NoError(t, err)
}

func TestRunManagedProcess_ComponentFailureCancelsSiblings(t *testing.T) {
	cfg := &config.Config{
		Server: &config.ServerConfig{
			Host:                    "127.0.0.1",
			GracefulShutdownTimeout: 2 * time.Second,
		},
	}

	siblingStopped := make(chan struct{})
	failingRunner := ComponentRunner{
		Name: "primary",
		Run: func(context.Context) error {
			return errors.New("boom")
		},
	}
	siblingRunner := ComponentRunner{
		Name: "sibling",
		Run: func(runCtx context.Context) error {
			<-runCtx.Done()
			close(siblingStopped)
			return nil
		},
	}

	err := runManagedProcess(context.Background(), cfg, discardCmdLogger(), nil, nil, failingRunner, siblingRunner)
	require.Error(t, err)
	assert.ErrorContains(t, err, "primary component failed: boom")

	select {
	case <-siblingStopped:
	case <-time.After(time.Second):
		t.Fatal("expected sibling component to stop after primary failure")
	}
}

func TestRunManagedProcess_TracerShutdownError(t *testing.T) {
	runner := ComponentRunner{
		Name: "one-off",
		Run: func(context.Context) error {
			return nil
		},
	}
	tracerShutdown := func(context.Context) error {
		return errors.New("flush failed")
	}

	err := runManagedProcess(context.Background(), nil, discardCmdLogger(), nil, tracerShutdown, runner)
	require.Error(t, err)
	assert.ErrorContains(t, err, "error shutting down tracer: flush failed")
}
