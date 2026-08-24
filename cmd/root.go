// Copyright 2026 Canonical Ltd.
// SPDX-License-Identifier: AGPL-3.0-only

package cmd

import (
	"encoding/json"
	"errors"
	"os"
	"time"

	"github.com/spf13/cobra"

	"github.com/canonical/authorization-service/cmd/authz"
	"github.com/canonical/authorization-service/config"
	"github.com/canonical/authorization-service/internal/version"
)

// rootCmd represents the base command when called without any subcommands
var rootCmd = &cobra.Command{
	Use:   "authz-service",
	Short: "Authorization Service",
	Long: `Authorization Service is a production-grade gRPC and REST API service
for managing authorization and permissions in microservices architectures.

It provides:
  - Versioned gRPC APIs with REST transcoding
  - Fine-grained authorization with OpenFGA
  - High-performance caching with Valkey
  - Kafka-based tuple ingestion
  - Istio/Envoy external authorization support`,
	Version: version.Version,
}

// Execute adds all child commands to the root command and sets flags appropriately.
// This is called by main.main(). It only needs to happen once to the rootCmd.
func Execute() {
	err := rootCmd.Execute()
	if err == nil {
		return
	}

	// Every RunE logs its own error with domain context before returning it,
	// except for a config.LoadConfig failure: it happens before cfg.Logging.SetupLogger()
	// So we print a hand-built JSON in order to never mix structured and unstructured output
	if errors.Is(err, config.ErrConfigLoad) {
		fallback := struct {
			Time  string `json:"time"`
			Level string `json:"level"`
			Msg   string `json:"msg"`
			Error string `json:"error"`
		}{
			Time:  time.Now().Format(time.RFC3339Nano),
			Level: "ERROR",
			Msg:   "fatal error before logger initialized",
			Error: err.Error(),
		}
		encoded, _ := json.Marshal(fallback)
		_, _ = os.Stderr.Write(append(encoded, '\n'))
	}

	os.Exit(1)
}

var configFile string

func init() {
	rootCmd.PersistentFlags().StringVarP(&configFile, "config", "c", "", "config file (default is ./authorization-service.yaml)")
	rootCmd.PersistentFlags().Int("grpc-port", 9091, "gRPC port")
	rootCmd.PersistentFlags().Int("http-port", 8070, "HTTP REST port")
	rootCmd.PersistentFlags().String("server-host", "0.0.0.0", "Server bind host")
	rootCmd.PersistentFlags().String("db-host", "localhost", "PostgreSQL database host")
	rootCmd.PersistentFlags().Int("db-port", 5432, "PostgreSQL database port")
	rootCmd.PersistentFlags().String("db-name", "authorization-service", "PostgreSQL database name")
	rootCmd.PersistentFlags().String("db-user", "authz", "PostgreSQL database user")
	rootCmd.PersistentFlags().Bool("dev", false, "Enable development mode")
	rootCmd.PersistentFlags().String("log-level", "info", "Logging level (debug, info, warn, error)")
	rootCmd.PersistentFlags().String("log-format", "json", "Logging format (json, text)")
	rootCmd.PersistentFlags().String("fga-address", "http://localhost:8081", "OpenFGA server address")
	rootCmd.PersistentFlags().String("fga-store-id", "", "OpenFGA store ID")
	rootCmd.PersistentFlags().String("fga-model-id", "", "OpenFGA authorization model ID")

	rootCmd.AddCommand(versionCmd)
	rootCmd.AddCommand(serveCmd)
	rootCmd.AddCommand(migrateCmd)
	rootCmd.AddCommand(authz.AuthzCmd)
	rootCmd.AddCommand(listenCmd)
	rootCmd.AddCommand(workerCmd)
	rootCmd.AddCommand(reaperCmd)
	rootCmd.AddCommand(seedCmd)
}
