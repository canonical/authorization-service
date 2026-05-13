package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/canonical/authorization-service/cmd/authz"
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
  - Event streaming with NATS JetStream
  - High-performance caching with Valkey
  - Istio/Envoy external authorization support`,
	Version: version.Version,
}

// Execute adds all child commands to the root command and sets flags appropriately.
// This is called by main.main(). It only needs to happen once to the rootCmd.
func Execute() {
	err := rootCmd.Execute()
	if err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

func init() {
	rootCmd.AddCommand(versionCmd)
	rootCmd.AddCommand(serveCmd)
	rootCmd.AddCommand(migrateCmd)
	rootCmd.AddCommand(authz.AuthzCmd)
	rootCmd.AddCommand(listenCmd)
}
