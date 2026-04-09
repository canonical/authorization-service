package cmd

import (
    "context"
    "fmt"
    "time"

    authv3 "github.com/envoyproxy/go-control-plane/envoy/service/auth/v3"
    typev3 "github.com/envoyproxy/go-control-plane/envoy/type/v3"
    "github.com/kelseyhightower/envconfig"
    "github.com/spf13/cobra"

    "github.com/canonical/authorization-service/config"
)

var (
    checkCookie string
    checkPath   string
    checkMethod string
    checkHost   string
)

// checkCmd represents the check command for testing Envoy authorization
var checkCmd = &cobra.Command{
    Use:   "check",
    Short: "Test the Envoy authorization Check method",
    Long: `Test the Envoy authorization Check method by simulating a request with a session cookie.

This command is useful for debugging and verifying the STS integration flow without
needing a full Envoy proxy setup.`,
    RunE: runCheck,
}

func init() {
    checkCmd.Flags().StringVar(&checkCookie, "cookie", "", "Session cookie value (e.g., 'session=abc123')")
    checkCmd.Flags().StringVar(&checkPath, "path", "/api/test", "Request path to check")
    checkCmd.Flags().StringVar(&checkMethod, "method", "GET", "HTTP method")
    checkCmd.Flags().StringVar(&checkHost, "host", "example.com", "Host header value")

    _ = checkCmd.MarkFlagRequired("cookie")

    rootCmd.AddCommand(checkCmd)
}

func runCheck(cmd *cobra.Command, args []string) error {
    ctx := context.Background()

    // Load configuration from environment variables
    cfg := &config.Config{}
    if err := envconfig.Process("", cfg); err != nil {
        return fmt.Errorf("failed to load configuration: %w", err)
    }

    logger := cfg.Logging.SetupLogger()

    tracer, _, err := cfg.Telemetry.SetupTelemetry(cmd.Context(), logger)
    if err != nil {
        return fmt.Errorf("failed to setup telemetry: %w", err)
    }

    // Initialize integrations and services
    integrations, err := initializeIntegrations(cfg, logger, tracer)
    if err != nil {
        return fmt.Errorf("failed to initialize integrations: %w", err)
    }
    defer integrations.cleanupIntegrations(logger)

    services, _ := integrations.initializeServices(tracer, logger)

    // Build a mock CheckRequest
    checkReq := &authv3.CheckRequest{
        Attributes: &authv3.AttributeContext{
            Request: &authv3.AttributeContext_Request{
                Http: &authv3.AttributeContext_HttpRequest{
                    Method: checkMethod,
                    Path:   checkPath,
                    Host:   checkHost,
                    Headers: map[string]string{
                        "cookie": checkCookie,
                        "host":   checkHost,
                    },
                    Protocol: "HTTP/1.1",
                },
            },
        },
    }

    logger.Info("Calling Check method",
        "method", checkMethod,
        "path", checkPath,
        "host", checkHost,
        "cookie", checkCookie,
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
