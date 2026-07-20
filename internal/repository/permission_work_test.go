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

	repo := NewPostgresPermissionWorkRepository(mockDB)
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

	repo := NewPostgresPermissionWorkRepository(mockDB)
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

	repo := NewPostgresPermissionWorkRepository(mockDB)
	err := repo.Insert(context.Background(), sampleRow())
	if err == nil || errors.Is(err, ErrDuplicate) {
		t.Fatalf("expected non-duplicate error, got %v", err)
	}
}
