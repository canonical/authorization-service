//go:build integration

package listener

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
	kafkaBroker string
	pgDSN       string
	pgConfig    postgres.Config
)

func TestMain(m *testing.M) {
	os.Exit(run(m))
}

func run(m *testing.M) int {
	ctx := context.Background()

	kafkaCtr, broker, err := suite.StartKafka(ctx)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to start Kafka: %v\n", err)
		return 1
	}
	defer testutil.StopContainer(ctx, kafkaCtr)
	kafkaBroker = broker

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

	if err := suite.CreateKafkaTopics(ctx, broker); err != nil {
		fmt.Fprintf(os.Stderr, "failed to create Kafka topics: %v\n", err)
		return 1
	}

	return m.Run()
}
