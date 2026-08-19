// Copyright 2026 Canonical Ltd.
// SPDX-License-Identifier: AGPL-3.0-only

package repository

import (
	"context"
	"errors"
	"fmt"
	"time"

	sq "github.com/Masterminds/squirrel"
	"github.com/google/uuid"
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
	db      postgres.DBClientInterface
	metrics Metrics
}

// NewPostgresPermissionWorkRepository creates a new PostgresPermissionWorkRepository.
// If metrics is nil a no-op is used.
func NewPostgresPermissionWorkRepository(db postgres.DBClientInterface, metrics Metrics) *PostgresPermissionWorkRepository {
	if metrics == nil {
		metrics = NoopMetrics{}
	}
	return &PostgresPermissionWorkRepository{db: db, metrics: metrics}
}

// Insert appends a row to permission_update_work in status 'received'.
func (r *PostgresPermissionWorkRepository) Insert(ctx context.Context, row permissions.WorkRow) (err error) {
	start := time.Now()
	defer func() { r.metrics.ObserveQuery("insert", err, time.Since(start)) }()

	id, err := uuid.NewV7()
	if err != nil {
		return fmt.Errorf("failed to generate UUIDv7: %w", err)
	}

	query, args, err := r.db.Builder().
		Insert("permission_update_work").
		Columns(
			"id",
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
			id.String(),
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
func (r *PostgresPermissionWorkRepository) ClaimBatch(ctx context.Context, limit int, retryAfter time.Duration) (claimed []permissions.ClaimedRow, err error) {
	start := time.Now()
	defer func() { r.metrics.ObserveQuery("claim_batch", err, time.Since(start)) }()

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

	claimed, err = scanClaimedRows(rows)
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
func (r *PostgresPermissionWorkRepository) RecordProcessed(ctx context.Context, id, service string, writes, deletes []permissions.Tuple) (err error) {
	start := time.Now()
	defer func() { r.metrics.ObserveQuery("record_processed", err, time.Since(start)) }()

	tx, err := r.db.Begin(ctx)
	if err != nil {
		return fmt.Errorf("failed to begin bookkeeping transaction: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	for _, t := range writes {
		baseSubject, userSetRelation := permissions.ParseSubject(t.Subject)

		var suffix string
		if userSetRelation == nil {
			suffix = "ON CONFLICT (subject, relation, object) WHERE user_set_subject_relation IS NULL DO NOTHING"
		} else {
			suffix = "ON CONFLICT (subject, user_set_subject_relation, relation, object) WHERE user_set_subject_relation IS NOT NULL DO NOTHING"
		}

		tupleID, err := uuid.NewV7()
		if err != nil {
			return fmt.Errorf("failed to generate UUIDv7 for authorization tuple: %w", err)
		}

		query, args, err := r.db.Builder().
			Insert("authorization_tuples").
			Columns("id", "service", "subject", "user_set_subject_relation", "relation", "object").
			Values(tupleID.String(), service, baseSubject, userSetRelation, t.Relation, t.Object).
			Suffix(suffix).
			ToSql()
		if err != nil {
			return fmt.Errorf("failed to build tuple insert: %w", err)
		}
		if _, err := tx.Exec(ctx, query, args...); err != nil {
			return fmt.Errorf("failed to insert authorization tuple: %w", err)
		}
	}

	for _, t := range deletes {
		baseSubject, userSetRelation := permissions.ParseSubject(t.Subject)

		eq := sq.Eq{
			"subject":  baseSubject,
			"relation": t.Relation,
			"object":   t.Object,
		}
		if userSetRelation == nil {
			eq["user_set_subject_relation"] = nil
		} else {
			eq["user_set_subject_relation"] = *userSetRelation
		}

		query, args, err := r.db.Builder().
			Delete("authorization_tuples").
			Where(eq).
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
func (r *PostgresPermissionWorkRepository) MarkFailed(ctx context.Context, id, errCode, errMsg string) (err error) {
	start := time.Now()
	defer func() { r.metrics.ObserveQuery("mark_failed", err, time.Since(start)) }()

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
func (r *PostgresPermissionWorkRepository) MarkRetry(ctx context.Context, id, errCode, errMsg string, incAttempt bool) (err error) {
	start := time.Now()
	defer func() { r.metrics.ObserveQuery("mark_retry", err, time.Since(start)) }()

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
// It uses a transaction-level advisory lock (pg_try_advisory_xact_lock) on a fixed key
// so that only one worker/reaper can execute reclaiming at any given time.
func (r *PostgresPermissionWorkRepository) ReclaimStale(ctx context.Context, staleAfter time.Duration) (count int64, err error) {
	start := time.Now()
	defer func() { r.metrics.ObserveQuery("reclaim_stale", err, time.Since(start)) }()

	// Start a transaction so the advisory lock is automatically released on commit/rollback.
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return 0, fmt.Errorf("failed to begin reclaim-stale transaction: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	// Fixed lock key for stale-row reaping (0x1D057 = 118871)
	const lockKey = 118871

	var acquired bool
	err = tx.QueryRow(ctx, "SELECT pg_try_advisory_xact_lock($1)", lockKey).Scan(&acquired)
	if err != nil {
		return 0, fmt.Errorf("failed to acquire advisory lock: %w", err)
	}
	if !acquired {
		// Lock is held by another process; exit gracefully with no-op.
		return 0, nil
	}

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

	tag, err := tx.Exec(ctx, query, args...)
	if err != nil {
		return 0, fmt.Errorf("failed to execute reclaim-stale update: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return 0, fmt.Errorf("failed to commit reclaim-stale transaction: %w", err)
	}

	return tag.RowsAffected(), nil
}

// intervalString renders a duration as a PostgreSQL interval literal (seconds),
// e.g. 5*time.Minute -> "300.000 seconds".
func intervalString(d time.Duration) string {
	return fmt.Sprintf("%f seconds", d.Seconds())
}
