// Copyright 2026 Canonical Ltd.
// SPDX-License-Identifier: AGPL-3.0-only

package cmd

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/canonical/authorization-service/config"
	"github.com/canonical/authorization-service/internal/metrics"
)

const defaultGracefulShutdownTimeout = 10 * time.Second

// ComponentRunner describes a named background component managed by runManagedProcess.
type ComponentRunner struct {
	Name string
	Run  func(ctx context.Context) error
}

// runManagedProcess starts the optional Prometheus metrics server and all provided
// ComponentRunner functions concurrently. It waits for SIGINT, SIGTERM, parent context
// cancellation, or the first component exit, then cancels the shared context, gracefully
// shuts down the metrics server, drains all remaining component goroutines, and executes
// tracerShutdown.
func runManagedProcess(
	ctx context.Context,
	cfg *config.Config,
	logger *slog.Logger,
	reg *prometheus.Registry,
	tracerShutdown func(context.Context) error,
	runners ...ComponentRunner,
) error {
	shutdownTimeout := defaultGracefulShutdownTimeout
	if cfg != nil && cfg.Server != nil && cfg.Server.GracefulShutdownTimeout > 0 {
		shutdownTimeout = cfg.Server.GracefulShutdownTimeout
	}

	var metricsServer *http.Server
	if cfg != nil && cfg.Metrics != nil && cfg.Metrics.Enabled && reg != nil {
		host := "0.0.0.0"
		if cfg.Server != nil && cfg.Server.Host != "" {
			host = cfg.Server.Host
		}
		metricsServer = metrics.NewServer(cfg.Metrics.GetAddress(host), cfg.Metrics.Path, reg)
	}

	runCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	numComponents := len(runners)
	if metricsServer != nil {
		numComponents++
	}

	type componentResult struct {
		name string
		err  error
	}

	errChan := make(chan componentResult, numComponents)
	for _, r := range runners {
		r := r
		go func() {
			errChan <- componentResult{name: r.Name, err: r.Run(runCtx)}
		}()
	}

	if metricsServer != nil {
		go func() {
			logger.InfoContext(runCtx, "Starting metrics server", "address", metricsServer.Addr)
			err := metricsServer.ListenAndServe()
			if errors.Is(err, http.ErrServerClosed) {
				err = nil
			} else if err != nil {
				logger.ErrorContext(runCtx, "Metrics server failed", "error", err)
			}
			errChan <- componentResult{name: "metrics", err: err}
		}()
	}

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
	defer signal.Stop(sigChan)

	remaining := numComponents
	var firstErr error

	if remaining > 0 {
		select {
		case res := <-errChan:
			remaining--
			logger.InfoContext(ctx, "Component stopped", "component", res.name)
			if res.err != nil {
				firstErr = fmt.Errorf("%s component failed: %w", res.name, res.err)
			}
			cancel()
		case sig := <-sigChan:
			logger.InfoContext(ctx, "Received signal, shutting down", "signal", sig)
			cancel()
		case <-ctx.Done():
			logger.InfoContext(ctx, "Context cancelled, shutting down")
			cancel()
		}
	}

	if metricsServer != nil {
		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), shutdownTimeout)
		if err := metricsServer.Shutdown(shutdownCtx); err != nil {
			logger.ErrorContext(ctx, "Error during metrics server shutdown", "error", err)
		}
		shutdownCancel()
	}

	for i := 0; i < remaining; i++ {
		res := <-errChan
		logger.InfoContext(ctx, "Component stopped", "component", res.name)
		if res.err != nil && firstErr == nil {
			firstErr = fmt.Errorf("%s component failed: %w", res.name, res.err)
		}
	}

	if tracerShutdown != nil {
		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), shutdownTimeout)
		defer shutdownCancel()

		if err := tracerShutdown(shutdownCtx); err != nil && firstErr == nil {
			firstErr = fmt.Errorf("error shutting down tracer: %w", err)
		}
	}

	return firstErr
}
