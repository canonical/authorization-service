// Copyright 2025 Canonical Ltd.
// SPDX-License-Identifier: AGPL-3.0

package authz

import (
    "context"
    _ "embed"
    "encoding/json"
    "fmt"
    "log/slog"
    "net/http"

    "github.com/kelseyhightower/envconfig"
    openfga "github.com/openfga/go-sdk"
    "github.com/openfga/go-sdk/client"
    "github.com/openfga/go-sdk/credentials"
    "github.com/openfga/language/pkg/go/transformer"
    "github.com/spf13/cobra"
    "google.golang.org/protobuf/encoding/protojson"

    "github.com/canonical/authorization-service/config"
)

//go:embed cerberus.v0.openfga
var v0AuthorizationModelDSL string

// WriteModelCmd represents the write-model command
var WriteModelCmd = &cobra.Command{
    Use:   "write-model <store-id>",
    Short: "Write the OpenFGA authorization model to the OpenFGA instance",
    Long: `Write the OpenFGA authorization model to the OpenFGA instance.
This command reads the authorization model from the embedded cerberus.v0.openfga file
and uploads it to the specified OpenFGA store.`,
    Args: cobra.ExactArgs(1),
    RunE: writeModel,
}

// writeModel is the handler for the write-model command
func writeModel(cmd *cobra.Command, args []string) error {
    storeID := args[0]

    cfg := &config.Config{}
    if err := envconfig.Process("", cfg); err != nil {
        return fmt.Errorf("failed to load configuration: %w", err)
    }

    // Setup logger
    logger := cfg.Logging.SetupLogger()

    if err := writeAuthorizationModel(cmd.Context(), storeID, cfg, logger); err != nil {
        logger.Error("Failed to write authorization model", "error", err)
        return err
    }

    return nil
}

// parseDSLToWriteRequest transforms the embedded DSL string into an SDK-compatible
// WriteAuthorizationModelRequest using the openfga language transformer.
func parseDSLToWriteRequest(dsl string) (*openfga.WriteAuthorizationModelRequest, error) {
    parsedAuthModel, err := transformer.TransformDSLToProto(dsl)
    if err != nil {
        return nil, fmt.Errorf("failed to transform DSL to proto: %w", err)
    }

    protoBytes, err := protojson.Marshal(parsedAuthModel)
    if err != nil {
        return nil, fmt.Errorf("failed to marshal proto to JSON: %w", err)
    }

    var request openfga.WriteAuthorizationModelRequest
    if err := json.Unmarshal(protoBytes, &request); err != nil {
        return nil, fmt.Errorf("failed to unmarshal into SDK request: %w", err)
    }

    return &request, nil
}

// writeAuthorizationModel writes the authorization model to OpenFGA
func writeAuthorizationModel(ctx context.Context, storeID string, cfg *config.Config, logger *slog.Logger) error {
    if !cfg.OpenFGA.Enabled {
        return fmt.Errorf("OpenFGA is not enabled in configuration")
    }

    if storeID == "" {
        return fmt.Errorf("store-id is required")
    }

    logger.Info("Writing authorization model to OpenFGA",
        "store_id", storeID,
        "address", cfg.OpenFGA.Address,
    )

    // Parse the embedded DSL model into a structured SDK request
    body, err := parseDSLToWriteRequest(v0AuthorizationModelDSL)
    if err != nil {
        return fmt.Errorf("failed to parse authorization model DSL: %w", err)
    }

    // Create OpenFGA SDK client
    clientConfig := &client.ClientConfiguration{
        ApiUrl:  cfg.OpenFGA.Address,
        StoreId: storeID,
        HTTPClient: &http.Client{
            Timeout: cfg.OpenFGA.Timeout,
        },
    }

    if cfg.OpenFGA.ApiKey != "" {
        clientConfig.Credentials = &credentials.Credentials{
            Method: credentials.CredentialsMethodApiToken,
            Config: &credentials.Config{
                ApiToken: cfg.OpenFGA.ApiKey,
            },
        }
    }

    fgaClient, err := client.NewSdkClient(clientConfig)
    if err != nil {
        return fmt.Errorf("failed to create OpenFGA client: %w", err)
    }

    // Write the authorization model via the SDK
    resp, err := fgaClient.WriteAuthorizationModel(ctx).Body(*body).Execute()
    if err != nil {
        return fmt.Errorf("failed to write authorization model: %w", err)
    }

    logger.Info("Authorization model written successfully",
        "store_id", storeID,
        "model_id", resp.GetAuthorizationModelId(),
    )

    return nil
}
