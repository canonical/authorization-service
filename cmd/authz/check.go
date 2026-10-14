// Copyright 2026 Canonical Ltd.
// SPDX-License-Identifier: AGPL-3.0-only

package authz

import (
	"context"
	"fmt"
	"time"

	authv3 "github.com/envoyproxy/go-control-plane/envoy/service/auth/v3"
	typev3 "github.com/envoyproxy/go-control-plane/envoy/type/v3"
	"github.com/spf13/cobra"

	"github.com/canonical/authorization-service/config"
	"github.com/canonical/authorization-service/internal/metrics"
	"github.com/canonical/authorization-service/internal/version"
)

var (
	checkCookie string
	checkToken  string
	checkPath   string
	checkMethod string
	checkHost   string
)

// checkCmd represents the check command for testing Envoy authorization
var checkCmd = &cobra.Command{
	Use:   "check",
	Short: "Test the Envoy authorization Check method",
	Long: `Test the Envoy authorization Check method by simulating a request with a session cookie or Bearer token.

This command is useful for debugging and verifying the STS integration flow without
needing a full Envoy proxy setup.`,
	RunE: runCheck,
}

func init() {
	checkCmd.Flags().StringVar(&checkCookie, "cookie", "", "Session cookie value (e.g., 'session=abc123')")
	checkCmd.Flags().StringVar(&checkToken, "token", "", "Bearer token value for machine-to-machine authentication")
	checkCmd.Flags().StringVar(&checkPath, "path", "/api/test", "Request path to check")
	checkCmd.Flags().StringVar(&checkMethod, "method", "GET", "HTTP method")
	checkCmd.Flags().StringVar(&checkHost, "host", "example.com", "Host header value")
}

func buildCheckRequest(method, path, host, cookie, token string) (*authv3.CheckRequest, error) {
	if cookie == "" && token == "" {
		return nil, fmt.Errorf("either --cookie or --token is required")
	}

	headers := map[string]string{
		"host": host,
	}
	if cookie != "" {
		headers["cookie"] = cookie
	}
	if token != "" {
		headers["authorization"] = "Bearer " + token
	}

	return &authv3.CheckRequest{
		Attributes: &authv3.AttributeContext{
			Request: &authv3.AttributeContext_Request{
				Http: &authv3.AttributeContext_HttpRequest{
					Method:   method,
					Path:     path,
					Host:     host,
					Headers:  headers,
					Protocol: "HTTP/1.1",
				},
			},
		},
	}, nil
}

func runCheck(cmd *cobra.Command, _ []string) error {
	ctx := cmd.Context()
	if ctx == nil {
		ctx = context.Background()
	}

	checkReq, err := buildCheckRequest(checkMethod, checkPath, checkHost, checkCookie, checkToken)
	if err != nil {
		return err
	}

	cfg, err := config.LoadConfigFor(cmd,
		config.ComponentServer,
		config.ComponentPostgres,
		config.ComponentOpenFGA,
		config.ComponentValkey,
		config.ComponentSTS,
		config.ComponentExtAuthz,
		config.ComponentLogging,
		config.ComponentTelemetry,
	)
	if err != nil {
		return err
	}

	logger := cfg.Logging.SetupLogger(cfg.Telemetry.ServiceName, version.Version)

	tracer, tracerShutdown, err := cfg.Telemetry.SetupTelemetry(ctx, logger)
	if err != nil {
		return fmt.Errorf("failed to setup telemetry: %w", err)
	}
	defer func() {
		shutdownTimeout := 10 * time.Second
		if cfg.Server != nil && cfg.Server.GracefulShutdownTimeout > 0 {
			shutdownTimeout = cfg.Server.GracefulShutdownTimeout
		}
		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), shutdownTimeout)
		defer shutdownCancel()
		if err := tracerShutdown(shutdownCtx); err != nil {
			logger.ErrorContext(ctx, "Failed to shut down tracer", "error", err)
		}
	}()

	// Initialize integrations and services
	integrations, err := config.InitializeIntegrations(
		cfg,
		logger,
		tracer,
		config.ComponentPostgres,
		config.ComponentOpenFGA,
		config.ComponentValkey,
		config.ComponentSTS,
	)
	if err != nil {
		return fmt.Errorf("failed to initialize integrations: %w", err)
	}
	defer integrations.CleanupIntegrations(logger)

	// This is a one-off debugging command, not continuously scraped, so the registry is discarded.
	services := integrations.InitializeServices(tracer, logger, metrics.NewRegistry())

	logger.InfoContext(ctx, "Calling Check method",
		"method", checkMethod,
		"path", checkPath,
		"host", checkHost,
		"has_cookie", checkCookie != "",
		"has_token", checkToken != "",
	)

	// Call the Check method
	checkCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	resp, err := services.ExternalAuthz.Check(checkCtx, checkReq)
	if err != nil {
		return fmt.Errorf("Check call failed: %w", err)
	}

	// Display the response
	fmt.Println("\n=== Check Response ===")
	fmt.Printf("Status Code: %d\n", resp.GetStatus().GetCode())
	fmt.Printf("Status Message: %s\n", resp.GetStatus().GetMessage())

	if okResp := resp.GetHttpResponse().(*authv3.CheckResponse_OkResponse); okResp != nil {
		fmt.Println("\n✅ Request ALLOWED")
		if len(okResp.OkResponse.GetHeaders()) > 0 {
			fmt.Println("\nHeaders to add to upstream request:")
			for _, header := range okResp.OkResponse.GetHeaders() {
				fmt.Printf("  %s: %s\n", header.GetHeader().GetKey(), header.GetHeader().GetValue())
			}
		}
	} else if deniedResp := resp.GetHttpResponse().(*authv3.CheckResponse_DeniedResponse); deniedResp != nil {
		fmt.Println("\n❌ Request DENIED")
		if status := deniedResp.DeniedResponse.GetStatus(); status != nil {
			statusCode := status.GetCode()
			statusName := typev3.StatusCode_name[int32(statusCode)]
			fmt.Printf("HTTP Status: %d (%s)\n", statusCode, statusName)
		}
		if body := deniedResp.DeniedResponse.GetBody(); body != "" {
			fmt.Printf("Response Body: %s\n", body)
		}
	}

	return nil
}
