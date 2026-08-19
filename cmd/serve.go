// Copyright 2026 Canonical Ltd.
// SPDX-License-Identifier: AGPL-3.0-only

package cmd

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"github.com/spf13/cobra"

	"github.com/canonical/authorization-service/config"
	"github.com/canonical/authorization-service/internal/metrics"
	"github.com/canonical/authorization-service/internal/version"
	"github.com/canonical/authorization-service/server/grpc"
	"github.com/canonical/authorization-service/server/rest"
)

// serveCmd represents the serve command
var serveCmd = &cobra.Command{
	Use:   "serve",
	Short: "Start the Authorization Service",
	Long: `Start the Authorization Service server.
The server will start both gRPC and REST API endpoints.
Configuration is loaded from environment variables.`,
	RunE: serve,
}

// serve is the main serve command handler
func serve(cmd *cobra.Command, args []string) error {
	cfg, err := config.LoadConfig(cmd)
	if err != nil {
		return err
	}

	// Setup logger
	logger := cfg.Logging.SetupLogger()
	logger.Info("Starting Authorization Service", "version", version.Version)

	tracer, tracerShutdown, err := cfg.Telemetry.SetupTelemetry(cmd.Context(), logger)
	if err != nil {
		logger.Error("Failed to setup telemetry", "error", err)
		return fmt.Errorf("telemetry setup failed: %w", err)
	}

	// Initialize integrations
	integrations, err := config.InitializeIntegrations(cfg, logger, tracer)
	if err != nil {
		logger.Error("Failed to initialize integrations", "error", err)
		return fmt.Errorf("integration initialization failed: %w", err)
	}

	defer integrations.CleanupIntegrations(logger)

	// Metrics registry and services
	reg := metrics.NewRegistry()
	services := integrations.InitializeServices(tracer, logger, reg)

	// Start gRPC server with functional options
	grpcServer, err := grpc.NewServer(
		cfg.Server,
		logger,
		reg,
		grpc.WithPermissionsService(services.Permissions),
		grpc.WithExternalAuthz(services.ExternalAuthz),
	)
	if err != nil {
		logger.Error("Failed to create gRPC server", "error", err)
		return fmt.Errorf("gRPC server creation failed: %w", err)
	}

	// Start REST gateway
	restGateway, err := rest.NewGateway(cfg.Server, reg, logger)
	if err != nil {
		logger.Error("Failed to create REST gateway", "error", err)
		return fmt.Errorf("REST gateway creation failed: %w", err)
	}

	var metricsServer *http.Server
	if cfg.Metrics.Enabled {
		metricsServer = metrics.NewServer(cfg.Metrics.GetAddress(cfg.Server.Host), cfg.Metrics.Path, reg)
	}

	// Start servers in goroutines
	errChan := make(chan error, 3)
	go func() {
		logger.Info("Starting gRPC server", "address", cfg.Server.GetGRPCAddress())
		if err := grpcServer.Start(); err != nil {
			errChan <- fmt.Errorf("gRPC server error: %w", err)
		}
	}()

	go func() {
		logger.Info("Starting REST gateway", "address", cfg.Server.GetHTTPAddress())
		if err := restGateway.Start(cfg.Server.GetGRPCAddress()); err != nil {
			errChan <- fmt.Errorf("REST gateway error: %w", err)
		}
	}()

	if metricsServer != nil {
		go func() {
			logger.Info("Starting metrics server", "address", metricsServer.Addr)
			if err := metricsServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
				errChan <- fmt.Errorf("metrics server error: %w", err)
			}
		}()
	}

	// Wait for termination signal or error
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
	select {
	case err := <-errChan:
		logger.Error("Server error", "error", err)
		return err
	case sig := <-sigChan:
		logger.Info("Received signal, shutting down", "signal", sig)
	}

	logger.Info("Shutting down servers...")

	ctx, cancel := context.WithTimeout(context.Background(), cfg.Server.GracefulShutdownTimeout)
	defer cancel()

	if err := tracerShutdown(ctx); err != nil {
		return fmt.Errorf("Error while shutting down tracer: %w", err)
	}

	grpcServer.GracefulStop()
	if err := restGateway.Shutdown(ctx); err != nil {
		logger.Error("Error during REST gateway shutdown", "error", err)
		return fmt.Errorf("shutdown error: %w", err)
	}
	if metricsServer != nil {
		if err := metricsServer.Shutdown(ctx); err != nil {
			logger.Error("Error during metrics server shutdown", "error", err)
		}
	}

	logger.Info("Authorization Service stopped")
	return nil
}
