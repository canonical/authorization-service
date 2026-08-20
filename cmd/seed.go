// Copyright 2026 Canonical Ltd.
// SPDX-License-Identifier: AGPL-3.0-only

package cmd

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/spf13/cobra"

	"github.com/canonical/authorization-service/authz/model"
	"github.com/canonical/authorization-service/config"
	"github.com/canonical/authorization-service/internal/integration/postgres"
	"github.com/canonical/authorization-service/internal/model/rules"
	"github.com/canonical/authorization-service/internal/repository"
	ruleservice "github.com/canonical/authorization-service/internal/service/rules"
	"github.com/canonical/authorization-service/internal/version"
)

var seedCmd = &cobra.Command{
	Use:   "seed",
	Short: "Seed route rules from YAML files",
	Long: `Seed route rules from versioned YAML files for federated services
into the database, comparing revisions naturally and replacing them atomically.`,
	RunE: runSeedCmd,
}

var validateCmd = &cobra.Command{
	Use:   "validate",
	Short: "Validate route rules YAML files",
	Long:  `Validate route rules YAML files without requiring database or environment configurations.`,
	RunE:  runValidateCmd,
}

func init() {
	seedCmd.Flags().StringP("dir", "d", "authz/model/services", "Directory to scan for rules.yaml files")
	seedCmd.Flags().Bool("dry-run", false, "Only validate route rules files without applying them to the database")

	validateCmd.Flags().StringP("dir", "d", "", "Directory to scan for rules.yaml files (if not specified, validates embedded rules)")
	seedCmd.AddCommand(validateCmd)
}

func runSeedCmd(cmd *cobra.Command, args []string) error {
	dryRun, _ := cmd.Flags().GetBool("dry-run")
	if dryRun {
		return runValidateCmd(cmd, args)
	}

	dirSpecified := cmd.Flags().Changed("dir")
	if dirSpecified {
		return fmt.Errorf("the --dir / -d flag can only be used during dry-run validation")
	}

	cfg, err := config.LoadConfig(cmd)
	if err != nil {
		return err
	}

	logger := cfg.Logging.SetupLogger(cfg.Telemetry.ServiceName, version.Version)
	logger.Info("Starting database seed command")

	tracer, tracerShutdown, err := cfg.Telemetry.SetupTelemetry(cmd.Context(), logger)
	if err != nil {
		return fmt.Errorf("telemetry setup failed: %w", err)
	}

	logger.Info("Scanning for embedded route rules")
	seedFiles, err := ruleservice.LoadSeedFilesFromFS(model.ModelFS, "services")
	if err != nil {
		logger.Error("Failed to scan embedded filesystem for rules.yaml files", "error", err)
		return err
	}

	if len(seedFiles) == 0 {
		logger.Info("No rules.yaml files found to seed")
		return nil
	}

	// Initialize Postgres Client manually to avoid needing other services (like Valkey/OpenFGA) to be online.
	pgClient, err := postgres.NewClient(
		postgres.Config{
			Host:            cfg.Postgres.Host,
			Port:            cfg.Postgres.Port,
			User:            cfg.Postgres.User,
			Password:        cfg.Postgres.Password,
			DBName:          cfg.Postgres.DBName,
			SSLMode:         cfg.Postgres.SSLMode,
			MaxOpenConns:    cfg.Postgres.MaxOpenConns,
			MaxIdleConns:    cfg.Postgres.MaxIdleConns,
			ConnMaxLifetime: cfg.Postgres.ConnMaxLifetime,
			ConnMaxIdleTime: cfg.Postgres.ConnMaxIdleTime,
			ConnectTimeout:  cfg.Postgres.ConnectTimeout,
		},
		logger,
		tracer,
	)
	if err != nil {
		logger.Error("Failed to initialize database client", "error", err)
		return fmt.Errorf("postgres client initialization failed: %w", err)
	}
	defer pgClient.Close()

	// This is a one-off batch job, not continuously scraped, so no metrics recorder is wired.
	repo := repository.NewPostgresRuleRepository(pgClient, nil)
	seeder := ruleservice.NewRuleSeeder(pgClient, repo)

	var succeeded, skipped, failed int
	for _, sf := range seedFiles {
		logger.Info("Processing rules file", "path", sf.FilePath, "service", sf.Service, "revision", sf.Revision)

		inserted, isSkipped, err := seeder.SeedService(cmd.Context(), sf)
		if err != nil {
			logger.Error("Failed to seed service", "service", sf.Service, "path", sf.FilePath, "error", err)
			failed++
			// Fail completely if any file fails validation or execution, rollback is handled per-service
			return fmt.Errorf("seeding failed for service %s: %w", sf.Service, err)
		}

		if isSkipped {
			logger.Info("Skipped seeding service (current or newer revision already exists in DB)", "service", sf.Service, "revision", sf.Revision)
			skipped++
		} else if inserted {
			logger.Info("Successfully seeded service", "service", sf.Service, "revision", sf.Revision)
			succeeded++
		}
	}

	logger.Info("Database seeding run completed", "total_files", len(seedFiles), "succeeded", succeeded, "skipped", skipped, "failed", failed)

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), cfg.Server.GracefulShutdownTimeout)
	defer shutdownCancel()

	if err := tracerShutdown(shutdownCtx); err != nil {
		return fmt.Errorf("error shutting down tracer: %w", err)
	}

	return nil
}

func runValidateCmd(cmd *cobra.Command, _ []string) error {
	logger := getLogger(cmd)
	logger.Info("Starting route rules validation")

	dir, _ := cmd.Flags().GetString("dir")
	dirSpecified := cmd.Flags().Changed("dir")

	var usePhysicalDir bool
	var scanDir string
	if cmd.Name() == "validate" {
		if dir != "" {
			usePhysicalDir = true
			scanDir = dir
		}
	} else {
		if dirSpecified {
			usePhysicalDir = true
			scanDir = dir
		}
	}

	var seedFiles []rules.SeedFile
	var err error

	if usePhysicalDir {
		logger.Info("Scanning for route rules in physical directory", "directory", scanDir)
		seedFiles, err = ruleservice.LoadSeedFiles(scanDir)
		if err != nil {
			logger.Error("Failed to scan directory for rules.yaml files", "error", err, "directory", scanDir)
			return err
		}
	} else {
		logger.Info("Scanning for embedded route rules")
		seedFiles, err = ruleservice.LoadSeedFilesFromFS(model.ModelFS, "services")
		if err != nil {
			logger.Error("Failed to scan embedded filesystem for rules.yaml files", "error", err)
			return err
		}
	}

	if len(seedFiles) == 0 {
		logger.Info("No rules.yaml files found to validate")
		return nil
	}

	var failedCount int
	for _, sf := range seedFiles {
		logger.Info("Validating rules file", "path", sf.FilePath, "service", sf.Service, "revision", sf.Revision)
		if err := ruleservice.ValidateSeedFile(sf); err != nil {
			logger.Error("Validation failed", "path", sf.FilePath, "service", sf.Service, "error", err)
			failedCount++
		} else {
			logger.Info("Validation succeeded", "path", sf.FilePath, "service", sf.Service, "revision", sf.Revision)
		}
	}

	if failedCount > 0 {
		return fmt.Errorf("validation failed for %d file(s)", failedCount)
	}

	logger.Info("Validation completed successfully")
	return nil
}

// getLogger builds a logger for runValidateCmd from CLI flags alone.
// validate must work "without requiring database or environment configurations"
// build just the LoggingConfig from the log-level/log-format flags instead, reusing the
// same SetupLogger as every other command.
func getLogger(cmd *cobra.Command) *slog.Logger {
	logLevel, _ := cmd.Flags().GetString("log-level")
	if logLevel == "" {
		logLevel, _ = cmd.Root().PersistentFlags().GetString("log-level")
	}
	logFormat, _ := cmd.Flags().GetString("log-format")
	if logFormat == "" {
		logFormat, _ = cmd.Root().PersistentFlags().GetString("log-format")
	}

	loggingCfg := &config.LoggingConfig{Level: logLevel, Format: logFormat}
	return loggingCfg.SetupLogger("authorization-service", version.Version)
}
