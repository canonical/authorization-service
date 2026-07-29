//go:build integration

package seeding

import (
	"context"
	"fmt"
	"os"
	"testing"

	"github.com/canonical/authorization-service/internal/integration/postgres"
	"github.com/canonical/authorization-service/internal/testutil"
	"github.com/canonical/authorization-service/tests/integration/suite"
)

var (
	pgDSN    string
	pgConfig postgres.Config
)

func TestMain(m *testing.M) {
	os.Exit(run(m))
}

func run(m *testing.M) int {
	ctx := context.Background()

	pgCtr, dsn, cfg, err := suite.StartPostgres(ctx)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to start Postgres: %v\n", err)
		return 1
	}
	defer testutil.StopContainer(ctx, pgCtr)
	pgDSN = dsn
	pgConfig = cfg

	if err := suite.RunMigrations(ctx, dsn); err != nil {
		fmt.Fprintf(os.Stderr, "failed to run migrations: %v\n", err)
		return 1
	}

	return m.Run()
}
