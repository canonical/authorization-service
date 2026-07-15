// Copyright 2026 Canonical Ltd.
// SPDX-License-Identifier: AGPL-3.0-only

package repository

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5/pgconn"

	"github.com/canonical/authorization-service/internal/integration/postgres"
	"github.com/canonical/authorization-service/internal/model/permissions"
)

// pgUniqueViolation is the PostgreSQL SQLSTATE for a unique_violation error.
const pgUniqueViolation = "23505"

// ErrDuplicate is returned by PermissionWorkRepository.Insert when a row with the
// same (service, idempotency_key) already exists. Callers should treat this as a
// successful, idempotent no-op rather than a failure.
var ErrDuplicate = errors.New("permission update already ingested")

// PermissionWorkRepository persists permission-update messages into the durable
// work table during ingestion.
type PermissionWorkRepository interface {
	// Insert appends a new row in status 'received'. It returns ErrDuplicate if a
	// row with the same (service, idempotency_key) already exists.
	Insert(ctx context.Context, row permissions.WorkRow) error
}

// Compile-time check to ensure PostgresPermissionWorkRepository implements the interface.
var _ PermissionWorkRepository = (*PostgresPermissionWorkRepository)(nil)

// PostgresPermissionWorkRepository implements PermissionWorkRepository using PostgreSQL.
type PostgresPermissionWorkRepository struct {
	db postgres.DBClientInterface
}

// NewPostgresPermissionWorkRepository creates a new PostgresPermissionWorkRepository.
func NewPostgresPermissionWorkRepository(db postgres.DBClientInterface) *PostgresPermissionWorkRepository {
	return &PostgresPermissionWorkRepository{db: db}
}

// Insert appends a row to permission_update_work in status 'received'.
func (r *PostgresPermissionWorkRepository) Insert(ctx context.Context, row permissions.WorkRow) error {
	query, args, err := r.db.Builder().
		Insert("permission_update_work").
		Columns(
			"service",
			"message_id",
			"idempotency_key",
			"version",
			"event_time",
			"ingestion_time",
			"correlation_id",
			"payload",
			"partition",
			"kafka_offset",
		).
		Values(
			row.Service,
			row.MessageID,
			row.IdempotencyKey,
			row.Version,
			row.EventTime,
			row.IngestionTime,
			row.CorrelationID,
			row.Payload,
			row.Partition,
			row.Offset,
		).
		ToSql()
	if err != nil {
		return fmt.Errorf("failed to build permission work insert: %w", err)
	}

	if _, err := r.db.Exec(ctx, query, args...); err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == pgUniqueViolation {
			return ErrDuplicate
		}
		return fmt.Errorf("failed to insert permission update work row: %w", err)
	}

	return nil
}
