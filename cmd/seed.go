// Copyright 2026 Canonical Ltd.
// SPDX-License-Identifier: AGPL-3.0-only

package cmd

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/canonical/authorization-service/authz/model"
	"github.com/canonical/authorization-service/config"
	"github.com/canonical/authorization-service/internal/integration/postgres"
	"github.com/canonical/authorization-service/internal/model/rules"
	"github.com/canonical/authorization-service/internal/repository"
	ruleservice "github.com/canonical/authorization-service/internal/service/rules"
)

var seedCmd = &cobra.Command{
	Use:   "seed",
	Short: "Seed route rules from YAML files",
	Long: `Seed route rules from versioned YAML files for federated services
into the database, comparing revisions naturally and replacing them atomically.`,
	RunE: runSeedCmd,
}

func init() {
	seedCmd.Flags().StringP("dir", "d", "authz/model/services", "Directory to scan for rules.yaml files")
	seedCmd.Flags().Bool("dry-run", false, "Only validate route rules files without applying them to the database")
}

func runSeedCmd(cmd *cobra.Command, _ []string) error {
	cfg, err := config.LoadConfig(cmd)
	if err != nil {
		return err
	}

	logger := cfg.Logging.SetupLogger()
	logger.Info("Starting database seed command")

	tracer, tracerShutdown, err := cfg.Telemetry.SetupTelemetry(cmd.Context(), logger)
	if err != nil {
		return fmt.Errorf("telemetry setup failed: %w", err)
	}

	dryRun, _ := cmd.Flags().GetBool("dry-run")
	dirSpecified := cmd.Flags().Changed("dir")

	if dirSpecified && !dryRun {
		return fmt.Errorf("the --dir / -d flag can only be used during dry-run validation")
	}

	var seedFiles []rules.SeedFile

	if dirSpecified {
		scanDir, _ := cmd.Flags().GetString("dir")
		logger.Info("Scanning for route rules in physical directory", "directory", scanDir)
		var err error
		seedFiles, err = ruleservice.LoadSeedFiles(scanDir)
		if err != nil {
			logger.Error("Failed to scan directory for rules.yaml files", "error", err, "directory", scanDir)
			return err
		}
	} else {
		logger.Info("Scanning for embedded route rules")
		var err error
		seedFiles, err = ruleservice.LoadSeedFilesFromFS(model.ModelFS, "services")
		if err != nil {
			logger.Error("Failed to scan embedded filesystem for rules.yaml files", "error", err)
			return err
		}
	}

	if len(seedFiles) == 0 {
		logger.Info("No rules.yaml files found to seed")
		return nil
	}

	if dryRun {
		logger.Info("Dry-run mode enabled. Performing only file validation.")
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

		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), cfg.Server.GracefulShutdownTimeout)
		defer shutdownCancel()

		if err := tracerShutdown(shutdownCtx); err != nil {
			return fmt.Errorf("error shutting down tracer: %w", err)
		}

		if failedCount > 0 {
			return fmt.Errorf("validation failed for %d file(s)", failedCount)
		}

		logger.Info("Dry-run validation completed successfully")
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

	repo := repository.NewPostgresRuleRepository(pgClient)
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
