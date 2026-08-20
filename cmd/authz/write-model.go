// Copyright 2025 Canonical Ltd.
// SPDX-License-Identifier: AGPL-3.0-only

package authz

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"path/filepath"

	openfga "github.com/openfga/go-sdk"
	"github.com/openfga/go-sdk/client"
	"github.com/openfga/go-sdk/credentials"
	"github.com/openfga/language/pkg/go/transformer"
	"github.com/spf13/cobra"
	"google.golang.org/protobuf/encoding/protojson"

	"github.com/canonical/authorization-service/authz/model"
	"github.com/canonical/authorization-service/config"
	"github.com/canonical/authorization-service/internal/version"
)

// writeModelCmd represents the write-model command
var writeModelCmd = &cobra.Command{
	Use:   "write-model <store-id>",
	Short: "Write the OpenFGA authorization model to the OpenFGA instance",
	Long: `Write the OpenFGA authorization model to the OpenFGA instance.
This command compiles the modular authorization model from the embedded authz/model/ core manifest (fga.mod)
and uploads it to the specified OpenFGA store.`,
	Args: cobra.ExactArgs(1),
	RunE: writeModel,
}

// writeModel is the handler for the write-model command
func writeModel(cmd *cobra.Command, args []string) error {
	storeID := args[0]

	cfg, err := config.LoadConfig(cmd)
	if err != nil {
		return err
	}

	// Setup logger
	logger := cfg.Logging.SetupLogger(cfg.Telemetry.ServiceName, version.Version)

	if err := WriteAuthorizationModel(cmd.Context(), storeID, cfg, logger); err != nil {
		logger.Error("Failed to write authorization model", "error", err)
		return err
	}

	return nil
}

// compileModularModel reads fga.mod from ModelFS, resolves its content files, and compiles
// them programmatically using the OpenFGA transformer package into a single authorization model.
func compileModularModel() (*openfga.WriteAuthorizationModelRequest, error) {
	// Read the manifest file fga.mod
	modData, err := model.ModelFS.ReadFile("fga.mod")
	if err != nil {
		return nil, fmt.Errorf("failed to read fga.mod from embedded FS: %w", err)
	}

	// Parse fga.mod
	modFile, err := transformer.TransformModFile(string(modData))
	if err != nil {
		return nil, fmt.Errorf("failed to parse fga.mod: %w", err)
	}

	schemaVersion := modFile.Schema.Value
	var modules []transformer.ModuleFile

	// Load each module file listed in the manifest
	for _, fileProp := range modFile.Contents.Value {
		relPath := fileProp.Value
		// Resolve the module path relative to the root folder
		fullPath := filepath.Clean(relPath)

		content, err := model.ModelFS.ReadFile(fullPath)
		if err != nil {
			return nil, fmt.Errorf("failed to read module file %s from embedded FS: %w", fullPath, err)
		}

		modules = append(modules, transformer.ModuleFile{
			Name:     relPath,
			Contents: string(content),
		})
	}

	// Compile modules into a single AuthorizationModel
	parsedAuthModel, err := transformer.TransformModuleFilesToModel(modules, schemaVersion)
	if err != nil {
		return nil, fmt.Errorf("failed to compile modular files to model: %w", err)
	}

	// Marshal compiled model to JSON, and then unmarshal it into SDK request
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

// WriteAuthorizationModel writes the authorization model to OpenFGA
func WriteAuthorizationModel(ctx context.Context, storeID string, cfg *config.Config, logger *slog.Logger) error {

	if storeID == "" {
		return fmt.Errorf("store-id is required")
	}

	logger.Info("Writing authorization model to OpenFGA",
		"store_id", storeID,
		"address", cfg.OpenFGA.Address,
	)

	// Compile the modular model from the embedded FS
	body, err := compileModularModel()
	if err != nil {
		return fmt.Errorf("failed to compile modular authorization model: %w", err)
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
