package cmd

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/kelseyhightower/envconfig"
	"github.com/spf13/cobra"

	"github.com/canonical/authorization-service/internal/config"
	"github.com/canonical/authorization-service/internal/server/grpc"
	"github.com/canonical/authorization-service/internal/server/rest"
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
	// Load configuration from environment variables
	cfg := &config.Config{}
	if err := envconfig.Process("", cfg); err != nil {
		return fmt.Errorf("failed to load configuration: %w", err)
	}

	// Setup logger
	logger := cfg.Logging.SetupLogger()
	logger.Info("Starting Authorization Service", "version", Version)

	tracer, tracerShutdown, err := cfg.Telemetry.SetupTelemetry(cmd.Context(), logger)
	if err != nil {
		logger.Error("Failed to setup telemetry", "error", err)
		return fmt.Errorf("telemetry setup failed: %w", err)
	}

	// Initialize integrations
	integrations, err := initializeIntegrations(cfg, logger)
	if err != nil {
		logger.Error("Failed to initialize integrations", "error", err)
		return fmt.Errorf("integration initialization failed: %w", err)
	}

	defer integrations.cleanupIntegrations()

	// Initialize services
	services := integrations.initializeServices(tracer, logger)

	// Start gRPC server with functional options
	grpcServer, err := grpc.NewServer(
		cfg.Server,
		logger,
		grpc.WithPermissionsService(services.Permissions),
		grpc.WithAuthzService(services.Authz),
		grpc.WithExternalAuthz(services.ExternalAuthz),
	)
	if err != nil {
		logger.Error("Failed to create gRPC server", "error", err)
		return fmt.Errorf("gRPC server creation failed: %w", err)
	}

	// Start REST gateway
	restGateway, err := rest.NewGateway(cfg.Server, logger)
	if err != nil {
		logger.Error("Failed to create REST gateway", "error", err)
		return fmt.Errorf("REST gateway creation failed: %w", err)
	}

	// Start servers in goroutines
	errChan := make(chan error, 2)
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

	// Graceful shutdown
	logger.Info("Shutting down servers...")

	if err := tracerShutdown(cmd.Context()); err != nil {
		return fmt.Errorf("Error while shutting down tracer: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), cfg.Server.GracefulShutdownTimeout)
	defer cancel()

	grpcServer.GracefulStop()
	if err := restGateway.Shutdown(ctx); err != nil {
		logger.Error("Error during REST gateway shutdown", "error", err)
		return fmt.Errorf("shutdown error: %w", err)
	}

	logger.Info("Authorization Service stopped")
	return nil
}
