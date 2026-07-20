// Copyright 2026 Canonical Ltd.
// SPDX-License-Identifier: AGPL-3.0-only

package repository

import (
	"context"
	"errors"
	"fmt"
	"time"

	sq "github.com/Masterminds/squirrel"
	"github.com/jackc/pgx/v5"
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
// work table during ingestion and drives their processing lifecycle in the worker.
type PermissionWorkRepository interface {
	// Insert appends a new row in status 'received'. It returns ErrDuplicate if a
	// row with the same (service, idempotency_key) already exists.
	Insert(ctx context.Context, row permissions.WorkRow) error

	// ClaimBatch atomically claims up to limit rows eligible for processing and
	// transitions them to 'processing'. A row is eligible when it is 'received'
	// and either was never attempted or its last attempt is older than retryAfter.
	// Rows are ordered by created_at and locked with FOR UPDATE SKIP LOCKED so
	// concurrent workers claim disjoint sets without blocking.
	ClaimBatch(ctx context.Context, limit int, retryAfter time.Duration) ([]permissions.ClaimedRow, error)

	// RecordProcessed records a successful application in a single short
	// transaction: it mirrors the applied writes/deletes into authorization_tuples
	// (tagged with the source service) and sets the work row to 'processed'. It is
	// called only after a successful, idempotent OpenFGA write.
	RecordProcessed(ctx context.Context, id, service string, writes, deletes []permissions.Tuple) error

	// MarkFailed moves a row to 'failed' (retry limit exhausted or permanent
	// error), incrementing attempt_count and recording the error.
	MarkFailed(ctx context.Context, id, errCode, errMsg string) error

	// MarkRetry returns a row to 'received' so it can be claimed again. When
	// incAttempt is true the failure occurred before or during the OpenFGA write
	// and counts towards the retry limit; when false it occurred during local
	// bookkeeping after a successful write and must not count against the limit.
	MarkRetry(ctx context.Context, id, errCode, errMsg string, incAttempt bool) error

	// ReclaimStale returns rows stuck in 'processing' beyond staleAfter (abandoned
	// by a crashed worker) to 'received' for reclaim. It returns the number of rows
	// reclaimed. Used by the stale-row reaper.
	ReclaimStale(ctx context.Context, staleAfter time.Duration) (int64, error)
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
		if pgErr, ok := errors.AsType[*pgconn.PgError](err); ok && pgErr.Code == pgUniqueViolation {
			return ErrDuplicate
		}
		return fmt.Errorf("failed to insert permission update work row: %w", err)
	}

	return nil
}

// ClaimBatch selects and locks eligible rows, transitions them to 'processing'
// and returns them. Selection and update happen in one transaction so a claim is
// atomic: another worker running the same query concurrently skips locked rows.
func (r *PostgresPermissionWorkRepository) ClaimBatch(ctx context.Context, limit int, retryAfter time.Duration) ([]permissions.ClaimedRow, error) {
	selectQuery, selectArgs, err := r.db.Builder().
		Select("id", "service", "message_id", "payload", "event_time", "attempt_count").
		From("permission_update_work").
		Where(sq.Eq{"status": string(permissions.StatusReceived)}).
		Where(sq.Expr(
			"(last_attempt_at IS NULL OR last_attempt_at < now() - ?::interval)",
			intervalString(retryAfter),
		)).
		OrderBy("created_at").
		Limit(uint64(limit)).
		Suffix("FOR UPDATE SKIP LOCKED").
		ToSql()
	if err != nil {
		return nil, fmt.Errorf("failed to build claim batch query: %w", err)
	}

	// Begin a transaction so the row locks taken by FOR UPDATE are held until the
	// status update is committed. Note: operations on the pgx.Tx bypass the
	// postgres client's tracing spans.
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to begin claim batch transaction: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	rows, err := tx.Query(ctx, selectQuery, selectArgs...)
	if err != nil {
		return nil, fmt.Errorf("failed to query claimable rows: %w", err)
	}

	claimed, err := scanClaimedRows(rows)
	if err != nil {
		return nil, err
	}
	if len(claimed) == 0 {
		return nil, nil
	}

	ids := make([]string, len(claimed))
	for i, row := range claimed {
		ids[i] = row.ID
	}

	updateQuery, updateArgs, err := r.db.Builder().
		Update("permission_update_work").
		Set("status", string(permissions.StatusProcessing)).
		Set("processing_started_at", sq.Expr("now()")).
		Where(sq.Expr("id = ANY(?)", ids)).
		ToSql()
	if err != nil {
		return nil, fmt.Errorf("failed to build claim update query: %w", err)
	}

	if _, err := tx.Exec(ctx, updateQuery, updateArgs...); err != nil {
		return nil, fmt.Errorf("failed to mark claimed rows as processing: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("failed to commit claim batch: %w", err)
	}

	return claimed, nil
}

// scanClaimedRows scans the claim query result into ClaimedRow values.
func scanClaimedRows(rows pgx.Rows) ([]permissions.ClaimedRow, error) {
	defer rows.Close()

	var claimed []permissions.ClaimedRow
	for rows.Next() {
		var (
			id           string
			service      string
			messageID    string
			payload      []byte
			eventTime    *time.Time
			attemptCount int
		)
		if err := rows.Scan(&id, &service, &messageID, &payload, &eventTime, &attemptCount); err != nil {
			return nil, fmt.Errorf("failed to scan claimed row: %w", err)
		}
		claimed = append(claimed, permissions.ClaimedRow{
			ID:           id,
			Service:      service,
			MessageID:    messageID,
			Payload:      payload,
			EventTime:    eventTime,
			AttemptCount: attemptCount,
		})
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating claimed rows: %w", err)
	}
	return claimed, nil
}

// RecordProcessed mirrors the applied tuples into authorization_tuples and marks
// the row 'processed', all in one transaction. Tuple upserts are idempotent
// (ON CONFLICT DO NOTHING) and deletes are naturally no-ops when absent, so a
// retry that re-runs bookkeeping after a prior partial success is safe.
func (r *PostgresPermissionWorkRepository) RecordProcessed(ctx context.Context, id, service string, writes, deletes []permissions.Tuple) error {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return fmt.Errorf("failed to begin bookkeeping transaction: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	for _, t := range writes {
		query, args, err := r.db.Builder().
			Insert("authorization_tuples").
			Columns("service", "subject", "relation", "object").
			Values(service, t.Subject, t.Relation, t.Object).
			Suffix("ON CONFLICT (subject, relation, object) DO NOTHING").
			ToSql()
		if err != nil {
			return fmt.Errorf("failed to build tuple insert: %w", err)
		}
		if _, err := tx.Exec(ctx, query, args...); err != nil {
			return fmt.Errorf("failed to insert authorization tuple: %w", err)
		}
	}

	for _, t := range deletes {
		query, args, err := r.db.Builder().
			Delete("authorization_tuples").
			Where(sq.Eq{"subject": t.Subject, "relation": t.Relation, "object": t.Object}).
			ToSql()
		if err != nil {
			return fmt.Errorf("failed to build tuple delete: %w", err)
		}
		if _, err := tx.Exec(ctx, query, args...); err != nil {
			return fmt.Errorf("failed to delete authorization tuple: %w", err)
		}
	}

	query, args, err := r.db.Builder().
		Update("permission_update_work").
		Set("status", string(permissions.StatusProcessed)).
		Set("processed_at", sq.Expr("now()")).
		Where(sq.Eq{"id": id}).
		ToSql()
	if err != nil {
		return fmt.Errorf("failed to build processed update: %w", err)
	}
	if _, err := tx.Exec(ctx, query, args...); err != nil {
		return fmt.Errorf("failed to mark row processed: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("failed to commit bookkeeping: %w", err)
	}
	return nil
}

// MarkFailed moves a row to 'failed', incrementing attempt_count and recording
// the error code and message.
func (r *PostgresPermissionWorkRepository) MarkFailed(ctx context.Context, id, errCode, errMsg string) error {
	var (
		codeVal *string = &errCode
		msgVal  *string = &errMsg
	)
	if errCode == "" {
		codeVal = nil
	}
	if errMsg == "" {
		msgVal = nil
	}

	query, args, err := r.db.Builder().
		Update("permission_update_work").
		Set("status", string(permissions.StatusFailed)).
		Set("attempt_count", sq.Expr("attempt_count + 1")).
		Set("last_attempt_at", sq.Expr("now()")).
		Set("last_error_code", codeVal).
		Set("last_error_message", msgVal).
		Where(sq.Eq{"id": id}).
		ToSql()
	if err != nil {
		return fmt.Errorf("failed to build mark-failed update: %w", err)
	}
	if _, err := r.db.Exec(ctx, query, args...); err != nil {
		return fmt.Errorf("failed to mark row failed: %w", err)
	}
	return nil
}

// MarkRetry returns a row to 'received'. incAttempt controls whether the failure
// counts towards the retry limit (true for pre/during-write failures, false for
// post-write bookkeeping failures, per the spec's retry model).
func (r *PostgresPermissionWorkRepository) MarkRetry(ctx context.Context, id, errCode, errMsg string, incAttempt bool) error {
	var (
		codeVal *string = &errCode
		msgVal  *string = &errMsg
	)
	if errCode == "" {
		codeVal = nil
	}
	if errMsg == "" {
		msgVal = nil
	}

	builder := r.db.Builder().
		Update("permission_update_work").
		Set("status", string(permissions.StatusReceived)).
		Set("last_attempt_at", sq.Expr("now()")).
		Set("last_error_code", codeVal).
		Set("last_error_message", msgVal).
		Set("processing_started_at", sq.Expr("NULL"))
	if incAttempt {
		builder = builder.Set("attempt_count", sq.Expr("attempt_count + 1"))
	}

	query, args, err := builder.Where(sq.Eq{"id": id}).ToSql()
	if err != nil {
		return fmt.Errorf("failed to build mark-retry update: %w", err)
	}
	if _, err := r.db.Exec(ctx, query, args...); err != nil {
		return fmt.Errorf("failed to mark row for retry: %w", err)
	}
	return nil
}

// ReclaimStale returns rows stuck in 'processing' beyond staleAfter to 'received'.
func (r *PostgresPermissionWorkRepository) ReclaimStale(ctx context.Context, staleAfter time.Duration) (int64, error) {
	query, args, err := r.db.Builder().
		Update("permission_update_work").
		Set("status", string(permissions.StatusReceived)).
		Set("processing_started_at", sq.Expr("NULL")).
		Where(sq.Eq{"status": string(permissions.StatusProcessing)}).
		Where(sq.Expr("processing_started_at < now() - ?::interval", intervalString(staleAfter))).
		ToSql()
	if err != nil {
		return 0, fmt.Errorf("failed to build reclaim-stale update: %w", err)
	}
	tag, err := r.db.Exec(ctx, query, args...)
	if err != nil {
		return 0, fmt.Errorf("failed to reclaim stale rows: %w", err)
	}
	return tag.RowsAffected(), nil
}

// intervalString renders a duration as a PostgreSQL interval literal (seconds),
// e.g. 5*time.Minute -> "300.000 seconds".
func intervalString(d time.Duration) string {
	return fmt.Sprintf("%f seconds", d.Seconds())
}
