//go:build integration

package migrations_test

import (
	"context"
	"database/sql"
	"testing"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/canonical/authorization-service/migrations"
	"github.com/canonical/authorization-service/internal/testutil"
	"github.com/canonical/authorization-service/tests/integration/suite"
)

// TestMigrations_Lifecycle verifies the full Up -> Down -> Up migration lifecycle
// on a clean PostgreSQL database, ensuring schema reversibility and idempotency.
func TestMigrations_Lifecycle(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	// 1. Start a fresh Postgres container (without running migrations automatically)
	pgCtr, dsn, _, err := suite.StartPostgres(ctx)
	require.NoError(t, err, "failed to start Postgres container")
	defer testutil.StopContainer(ctx, pgCtr)

	db, err := sql.Open("pgx/v5", dsn)
	require.NoError(t, err)
	defer db.Close()

	require.NoError(t, db.PingContext(ctx))

	goose.SetBaseFS(migrations.EmbedMigrations)
	require.NoError(t, goose.SetDialect("postgres"))

	// --- Phase 1: Migrate UP to latest ---
	t.Log("Phase 1: Running goose UpContext to latest version...")
	err = goose.UpContext(ctx, db, ".", goose.WithNoColor(true))
	require.NoError(t, err, "goose UpContext failed")

	expectedTables := []string{
		"federated_service",
		"authorization_rule",
		"authorization_rule_tuple",
		"permission_update_work",
		"authorization_tuples",
	}

	for _, tbl := range expectedTables {
		var exists bool
		query := `SELECT EXISTS (
			SELECT FROM information_schema.tables 
			WHERE table_schema = 'public' AND table_name = $1
		)`
		err := db.QueryRowContext(ctx, query, tbl).Scan(&exists)
		require.NoError(t, err)
		assert.True(t, exists, "table %s should exist after goose Up", tbl)
	}

	// Verify custom enum types exist
	expectedEnums := []string{"http_method", "permission_update_status"}
	for _, enumName := range expectedEnums {
		var exists bool
		query := `SELECT EXISTS (
			SELECT 1 FROM pg_type WHERE typname = $1
		)`
		err := db.QueryRowContext(ctx, query, enumName).Scan(&exists)
		require.NoError(t, err)
		assert.True(t, exists, "enum type %s should exist after goose Up", enumName)
	}

	// --- Phase 2: Migrate DOWN to 0 (revert all migrations) ---
	t.Log("Phase 2: Running goose DownToContext to version 0...")
	err = goose.DownToContext(ctx, db, ".", 0, goose.WithNoColor(true))
	require.NoError(t, err, "goose DownToContext to 0 failed")

	for _, tbl := range expectedTables {
		var exists bool
		query := `SELECT EXISTS (
			SELECT FROM information_schema.tables 
			WHERE table_schema = 'public' AND table_name = $1
		)`
		err := db.QueryRowContext(ctx, query, tbl).Scan(&exists)
		require.NoError(t, err)
		assert.False(t, exists, "table %s should NOT exist after goose Down to 0", tbl)
	}

	for _, enumName := range expectedEnums {
		var exists bool
		query := `SELECT EXISTS (
			SELECT 1 FROM pg_type WHERE typname = $1
		)`
		err := db.QueryRowContext(ctx, query, enumName).Scan(&exists)
		require.NoError(t, err)
		assert.False(t, exists, "enum type %s should NOT exist after goose Down to 0", enumName)
	}

	// --- Phase 3: Re-migrate UP to latest (verify idempotency and re-creation) ---
	t.Log("Phase 3: Re-running goose UpContext to latest version...")
	err = goose.UpContext(ctx, db, ".", goose.WithNoColor(true))
	require.NoError(t, err, "goose Re-Up failed")

	// Verify tables are re-created and usable by executing a test insert
	_, err = db.ExecContext(ctx, `
		INSERT INTO federated_service (id, slug, description, tenant)
		VALUES ('00000000-0000-0000-0000-000000000001', 'test-service', 'Test Description', 'Canonical')
	`)
	require.NoError(t, err, "insert into federated_service failed after Re-Up")
}
