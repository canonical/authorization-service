// Copyright 2026 Canonical Ltd.
// SPDX-License-Identifier: AGPL-3.0-only

package repository

import (
	"context"
	"errors"
	"testing"
	"time"

	sq "github.com/Masterminds/squirrel"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/pashagolub/pgxmock/v5"
	"go.uber.org/mock/gomock"

	"github.com/canonical/authorization-service/internal/model/permissions"
	repomocks "github.com/canonical/authorization-service/internal/repository/mocks"
)

func setupWorkMocks(t *testing.T) (*repomocks.MockDBClientInterface, pgxmock.PgxPoolIface) {
	t.Helper()
	ctrl := gomock.NewController(t)
	mockDB := repomocks.NewMockDBClientInterface(ctrl)
	pool, err := pgxmock.NewPool()
	if err != nil {
		t.Fatalf("failed to create pgxmock pool: %v", err)
	}
	t.Cleanup(func() { pool.Close() })

	mockDB.EXPECT().Builder().Return(sq.StatementBuilder.PlaceholderFormat(sq.Dollar)).AnyTimes()
	return mockDB, pool
}

func sampleRow() permissions.WorkRow {
	return permissions.WorkRow{
		Service:        "payments",
		MessageID:      "msg-1",
		IdempotencyKey: "idem-1",
		Version:        "1",
		IngestionTime:  time.Unix(0, 0).UTC(),
		Payload:        []byte{0x01},
	}
}

func TestPermissionWork_Insert_Success(t *testing.T) {
	mockDB, pool := setupWorkMocks(t)

	mockDB.EXPECT().
		Exec(gomock.Any(), gomock.Any(), gomock.Any()).
		DoAndReturn(func(ctx context.Context, query string, args ...interface{}) (interface{}, error) {
			pool.ExpectExec(".*").WillReturnResult(pgxmock.NewResult("INSERT", 1))
			return pool.Exec(ctx, "test")
		})

	repo := NewPostgresPermissionWorkRepository(mockDB, nil)
	if err := repo.Insert(context.Background(), sampleRow()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestPermissionWork_Insert_Duplicate(t *testing.T) {
	mockDB, pool := setupWorkMocks(t)

	mockDB.EXPECT().
		Exec(gomock.Any(), gomock.Any(), gomock.Any()).
		DoAndReturn(func(ctx context.Context, query string, args ...interface{}) (interface{}, error) {
			pool.ExpectExec(".*").WillReturnError(&pgconn.PgError{Code: pgUniqueViolation})
			return pool.Exec(ctx, "test")
		})

	repo := NewPostgresPermissionWorkRepository(mockDB, nil)
	err := repo.Insert(context.Background(), sampleRow())
	if !errors.Is(err, ErrDuplicate) {
		t.Fatalf("expected ErrDuplicate, got %v", err)
	}
}

func TestPermissionWork_Insert_OtherError(t *testing.T) {
	mockDB, pool := setupWorkMocks(t)

	mockDB.EXPECT().
		Exec(gomock.Any(), gomock.Any(), gomock.Any()).
		DoAndReturn(func(ctx context.Context, query string, args ...interface{}) (interface{}, error) {
			pool.ExpectExec(".*").WillReturnError(errors.New("connection refused"))
			return pool.Exec(ctx, "test")
		})

	repo := NewPostgresPermissionWorkRepository(mockDB, nil)
	err := repo.Insert(context.Background(), sampleRow())
	if err == nil || errors.Is(err, ErrDuplicate) {
		t.Fatalf("expected non-duplicate error, got %v", err)
	}
}

func TestPermissionWork_ClaimBatch_Success(t *testing.T) {
	mockDB, pool := setupWorkMocks(t)

	// The claim runs SELECT ... FOR UPDATE SKIP LOCKED then UPDATE ... in one tx.
	mockDB.EXPECT().
		Begin(gomock.Any()).
		DoAndReturn(func(ctx context.Context) (interface{}, error) {
			pool.ExpectBegin()
			pool.ExpectQuery(".*").
				WithArgs(pgxmock.AnyArg(), pgxmock.AnyArg()).
				WillReturnRows(
					pgxmock.NewRows([]string{"id", "service", "message_id", "payload", "event_time", "attempt_count"}).
						AddRow("row-1", "payments", "msg-1", []byte{0x01}, (*time.Time)(nil), 0).
						AddRow("row-2", "payments", "msg-2", []byte{0x02}, (*time.Time)(nil), 2),
				)
			pool.ExpectExec(".*").
				WithArgs(pgxmock.AnyArg(), pgxmock.AnyArg()).
				WillReturnResult(pgxmock.NewResult("UPDATE", 2))
			pool.ExpectCommit()
			return pool.Begin(ctx)
		})

	repo := NewPostgresPermissionWorkRepository(mockDB, nil)
	rows, err := repo.ClaimBatch(context.Background(), 20, 5*time.Minute)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("expected 2 claimed rows, got %d", len(rows))
	}
	if rows[0].ID != "row-1" || rows[1].AttemptCount != 2 {
		t.Fatalf("unexpected claimed rows: %+v", rows)
	}
}

func TestPermissionWork_ClaimBatch_Empty(t *testing.T) {
	mockDB, pool := setupWorkMocks(t)

	// No eligible rows: the claim commits without issuing the UPDATE.
	mockDB.EXPECT().
		Begin(gomock.Any()).
		DoAndReturn(func(ctx context.Context) (interface{}, error) {
			pool.ExpectBegin()
			pool.ExpectQuery(".*").
				WithArgs(pgxmock.AnyArg(), pgxmock.AnyArg()).
				WillReturnRows(
					pgxmock.NewRows([]string{"id", "service", "message_id", "payload", "event_time", "attempt_count"}),
				)
			pool.ExpectRollback()
			return pool.Begin(ctx)
		})

	repo := NewPostgresPermissionWorkRepository(mockDB, nil)
	rows, err := repo.ClaimBatch(context.Background(), 20, 5*time.Minute)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(rows) != 0 {
		t.Fatalf("expected no claimed rows, got %d", len(rows))
	}
}

func TestPermissionWork_RecordProcessed_Success(t *testing.T) {
	mockDB, pool := setupWorkMocks(t)

	mockDB.EXPECT().
		Begin(gomock.Any()).
		DoAndReturn(func(ctx context.Context) (interface{}, error) {
			pool.ExpectBegin()
			// Direct subject write (6 args including id)
			pool.ExpectExec("INSERT INTO authorization_tuples").
				WithArgs(pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg()).
				WillReturnResult(pgxmock.NewResult("INSERT", 1))
			// Userset subject write (6 args including id)
			pool.ExpectExec("INSERT INTO authorization_tuples").
				WithArgs(pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg()).
				WillReturnResult(pgxmock.NewResult("INSERT", 1))

			// Direct subject delete (3 args, because IS NULL does not use a placeholder)
			pool.ExpectExec("DELETE FROM authorization_tuples").
				WithArgs(pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg()).
				WillReturnResult(pgxmock.NewResult("DELETE", 1))
			// Userset subject delete (4 args)
			pool.ExpectExec("DELETE FROM authorization_tuples").
				WithArgs(pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg()).
				WillReturnResult(pgxmock.NewResult("DELETE", 1))

			pool.ExpectExec("UPDATE permission_update_work").
				WithArgs(pgxmock.AnyArg(), pgxmock.AnyArg()).
				WillReturnResult(pgxmock.NewResult("UPDATE", 1))
			pool.ExpectCommit()
			return pool.Begin(ctx)
		})

	repo := NewPostgresPermissionWorkRepository(mockDB, nil)
	err := repo.RecordProcessed(context.Background(), "row-1", "payments",
		[]permissions.Tuple{
			{Subject: "user:u1", Relation: "viewer", Object: "doc:d1"},
			{Subject: "role:admin#assignee", Relation: "reader", Object: "doc:d3"},
		},
		[]permissions.Tuple{
			{Subject: "user:u2", Relation: "editor", Object: "doc:d2"},
			{Subject: "role:member#assignee", Relation: "editor", Object: "doc:d4"},
		},
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestPermissionWork_MarkFailed(t *testing.T) {
	mockDB, pool := setupWorkMocks(t)

	mockDB.EXPECT().
		Exec(gomock.Any(), gomock.Any(), gomock.Any()).
		DoAndReturn(func(ctx context.Context, query string, args ...interface{}) (interface{}, error) {
			pool.ExpectExec(".*").WillReturnResult(pgxmock.NewResult("UPDATE", 1))
			return pool.Exec(ctx, "test")
		})

	repo := NewPostgresPermissionWorkRepository(mockDB, nil)
	if err := repo.MarkFailed(context.Background(), "row-1", "openfga_write_rejected", "bad input"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestPermissionWork_MarkRetry(t *testing.T) {
	tests := []struct {
		name       string
		incAttempt bool
	}{
		{"pre-write failure increments attempts", true},
		{"post-write bookkeeping failure does not", false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			mockDB, pool := setupWorkMocks(t)

			mockDB.EXPECT().
				Exec(gomock.Any(), gomock.Any(), gomock.Any()).
				DoAndReturn(func(ctx context.Context, query string, args ...interface{}) (interface{}, error) {
					pool.ExpectExec(".*").WillReturnResult(pgxmock.NewResult("UPDATE", 1))
					return pool.Exec(ctx, "test")
				})

			repo := NewPostgresPermissionWorkRepository(mockDB, nil)
			if err := repo.MarkRetry(context.Background(), "row-1", "code", "msg", tc.incAttempt); err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}

func TestPermissionWork_ReclaimStale(t *testing.T) {
	mockDB, pool := setupWorkMocks(t)

	mockDB.EXPECT().
		Begin(gomock.Any()).
		DoAndReturn(func(ctx context.Context) (interface{}, error) {
			pool.ExpectBegin()
			pool.ExpectQuery("SELECT pg_try_advisory_xact_lock.*").
				WithArgs(118871).
				WillReturnRows(pgxmock.NewRows([]string{"acquired"}).AddRow(true))
			pool.ExpectExec("UPDATE permission_update_work.*").
				WithArgs(string(permissions.StatusReceived), string(permissions.StatusProcessing), pgxmock.AnyArg()).
				WillReturnResult(pgxmock.NewResult("UPDATE", 3))
			pool.ExpectCommit()
			return pool.Begin(ctx)
		})

	repo := NewPostgresPermissionWorkRepository(mockDB, nil)
	n, err := repo.ReclaimStale(context.Background(), 15*time.Minute)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if n != 3 {
		t.Fatalf("expected 3 reclaimed rows, got %d", n)
	}
}
