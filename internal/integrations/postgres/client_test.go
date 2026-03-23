package postgres

import (
    "context"
    "errors"
    "testing"
    "time"

    "github.com/pashagolub/pgxmock/v5"

    "github.com/canonical/authorization-service/internal/testutil"
)

// newTestClient builds a Client with the given mock pool, bypassing NewClient's
// real connection logic.
func newTestClient(t *testing.T, pool poolInterface) *Client {
    t.Helper()

    return &Client{
        pool:    pool,
        builder: builderWithDollar(),
        tracer:  testutil.TestTracer(t),
        logger:  testutil.TestLogger(t),
    }
}

// newMockPool creates a pgxmock pool and registers cleanup.
func newMockPool(t *testing.T) pgxmock.PgxPoolIface {
    t.Helper()
    mock, err := pgxmock.NewPool()
    if err != nil {
        t.Fatalf("failed to create pgxmock pool: %v", err)
    }
    t.Cleanup(func() { mock.Close() })
    return mock
}

// ── Config ────────────────────────────────────────────────────────────────────

func TestConfig_DSN(t *testing.T) {
    cfg := Config{
        Host:           "localhost",
        Port:           5432,
        User:           "user",
        Password:       "pass",
        DBName:         "authz",
        SSLMode:        "disable",
        ConnectTimeout: 10 * time.Second,
    }

    dsn := cfg.DSN()
    expected := "host=localhost port=5432 user=user password=pass dbname=authz sslmode=disable connect_timeout=10"
    if dsn != expected {
        t.Errorf("expected DSN %q, got %q", expected, dsn)
    }
}

func TestConfig_DSN_ZeroTimeout(t *testing.T) {
    cfg := Config{
        Host:           "db",
        Port:           5432,
        User:           "u",
        DBName:         "d",
        SSLMode:        "require",
        ConnectTimeout: time.Duration(0),
    }

    dsn := cfg.DSN()
    expected := "host=db port=5432 user=u password= dbname=d sslmode=require connect_timeout=0"
    if dsn != expected {
        t.Errorf("expected DSN %q, got %q", expected, dsn)
    }
}

// ── Offset & PageSize ─────────────────────────────────────────────────────────

func TestOffset(t *testing.T) {
    tests := []struct {
        name     string
        page     int64
        pageSize uint64
        expected uint64
    }{
        {"first page", 1, 10, 0},
        {"second page", 2, 10, 10},
        {"third page", 3, 25, 50},
        {"zero page falls back to page 1", 0, 10, 0},
        {"negative page falls back to page 1", -5, 10, 0},
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            got := Offset(tt.page, tt.pageSize)
            if got != tt.expected {
                t.Errorf("Offset(%d, %d) = %d, want %d", tt.page, tt.pageSize, got, tt.expected)
            }
        })
    }
}

func TestPageSize(t *testing.T) {
    tests := []struct {
        name     string
        input    int64
        expected uint64
    }{
        {"positive value", 50, 50},
        {"one", 1, 1},
        {"zero falls back to default", 0, defaultPageSize},
        {"negative falls back to default", -1, defaultPageSize},
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            got := PageSize(tt.input)
            if got != tt.expected {
                t.Errorf("PageSize(%d) = %d, want %d", tt.input, got, tt.expected)
            }
        })
    }
}

// ── Builder ───────────────────────────────────────────────────────────────────

func TestClient_Builder(t *testing.T) {
    mock := newMockPool(t)
    client := newTestClient(t, mock)
    b := client.Builder()

    // Dollar placeholders must be used
    sql, args, err := b.Select("id").From("users").Where("id = ?", 42).ToSql()
    if err != nil {
        t.Fatalf("ToSql failed: %v", err)
    }
    if sql != "SELECT id FROM users WHERE id = $1" {
        t.Errorf("unexpected SQL: %q", sql)
    }
    if len(args) != 1 || args[0] != 42 {
        t.Errorf("unexpected args: %v", args)
    }
}

// ── Query ─────────────────────────────────────────────────────────────────────

func TestClient_Query_Success(t *testing.T) {

    mock := newMockPool(t)

    rows := mock.NewRows([]string{"id"}).AddRow(1)
    mock.ExpectQuery("SELECT 1").WillReturnRows(rows)

    client := newTestClient(t, mock)
    result, err := client.Query(context.Background(), "SELECT 1")

    if err != nil {
        t.Fatalf("expected no error, got %v", err)
    }
    if result == nil {
        t.Error("expected non-nil rows")
    }

    if err := mock.ExpectationsWereMet(); err != nil {
        t.Errorf("unfulfilled expectations: %v", err)
    }
}

func TestClient_Query_Error(t *testing.T) {
    mock := newMockPool(t)
    dbErr := errors.New("connection reset")

    mock.ExpectQuery("SELECT 1").WillReturnError(dbErr)

    client := newTestClient(t, mock)
    result, err := client.Query(context.Background(), "SELECT 1")

    if err == nil {
        t.Fatal("expected an error, got nil")
    }
    if result != nil {
        t.Error("expected nil rows on error")
    }
    if !errors.Is(err, dbErr) {
        t.Errorf("expected error to wrap %v, got %v", dbErr, err)
    }

    if err := mock.ExpectationsWereMet(); err != nil {
        t.Errorf("unfulfilled expectations: %v", err)
    }
}

func TestClient_Query_WithArgs(t *testing.T) {
    mock := newMockPool(t)

    rows := mock.NewRows([]string{"id", "name"})
    mock.ExpectQuery("SELECT \\* FROM t WHERE id = \\$1").
        WithArgs(99).
        WillReturnRows(rows)

    client := newTestClient(t, mock)
    _, err := client.Query(context.Background(), "SELECT * FROM t WHERE id = $1", 99)
    if err != nil {
        t.Fatalf("unexpected error: %v", err)
    }

    if err := mock.ExpectationsWereMet(); err != nil {
        t.Errorf("unfulfilled expectations: %v", err)
    }
}

// ── QueryRow ──────────────────────────────────────────────────────────────────

func TestClient_QueryRow_Success(t *testing.T) {
    mock := newMockPool(t)

    rows := mock.NewRows([]string{"result"}).AddRow(1)
    mock.ExpectQuery("SELECT 1").WillReturnRows(rows)

    client := newTestClient(t, mock)
    row := client.QueryRow(context.Background(), "SELECT 1")

    if row == nil {
        t.Error("expected non-nil row")
    }

    var result int
    if err := row.Scan(&result); err != nil {
        t.Fatalf("Scan failed: %v", err)
    }
    if result != 1 {
        t.Errorf("expected 1, got %d", result)
    }

    if err := mock.ExpectationsWereMet(); err != nil {
        t.Errorf("unfulfilled expectations: %v", err)
    }
}

func TestClient_QueryRow_WithArgs(t *testing.T) {
    mock := newMockPool(t)

    rows := mock.NewRows([]string{"id"}).AddRow(7)
    mock.ExpectQuery("SELECT id FROM t WHERE id = \\$1").
        WithArgs(7).
        WillReturnRows(rows)

    client := newTestClient(t, mock)
    row := client.QueryRow(context.Background(), "SELECT id FROM t WHERE id = $1", 7)
    if row == nil {
        t.Error("expected non-nil row")
    }

    if err := mock.ExpectationsWereMet(); err != nil {
        t.Errorf("unfulfilled expectations: %v", err)
    }
}

// ── Exec ──────────────────────────────────────────────────────────────────────

func TestClient_Exec_Success(t *testing.T) {
    mock := newMockPool(t)

    mock.ExpectExec("INSERT INTO t VALUES \\(\\$1\\)").
        WithArgs(1).
        WillReturnResult(pgxmock.NewResult("INSERT", 1))

    client := newTestClient(t, mock)
    tag, err := client.Exec(context.Background(), "INSERT INTO t VALUES ($1)", 1)

    if err != nil {
        t.Fatalf("expected no error, got %v", err)
    }
    if tag.RowsAffected() != 1 {
        t.Errorf("expected 1 row affected, got %d", tag.RowsAffected())
    }

    if err := mock.ExpectationsWereMet(); err != nil {
        t.Errorf("unfulfilled expectations: %v", err)
    }
}

func TestClient_Exec_Error(t *testing.T) {
    mock := newMockPool(t)
    dbErr := errors.New("unique constraint violation")

    mock.ExpectExec("INSERT INTO t VALUES \\(\\$1\\)").
        WithArgs(1).
        WillReturnError(dbErr)

    client := newTestClient(t, mock)
    _, err := client.Exec(context.Background(), "INSERT INTO t VALUES ($1)", 1)

    if err == nil {
        t.Fatal("expected an error, got nil")
    }
    if !errors.Is(err, dbErr) {
        t.Errorf("expected error to wrap %v, got %v", dbErr, err)
    }

    if err := mock.ExpectationsWereMet(); err != nil {
        t.Errorf("unfulfilled expectations: %v", err)
    }
}

// ── Begin ─────────────────────────────────────────────────────────────────────

func TestClient_Begin_Success(t *testing.T) {
    mock := newMockPool(t)

    mock.ExpectBegin()

    client := newTestClient(t, mock)
    tx, err := client.Begin(context.Background())

    if err != nil {
        t.Fatalf("expected no error, got %v", err)
    }
    if tx == nil {
        t.Error("expected non-nil tx")
    }

    if err := mock.ExpectationsWereMet(); err != nil {
        t.Errorf("unfulfilled expectations: %v", err)
    }
}

func TestClient_Begin_Error(t *testing.T) {
    mock := newMockPool(t)
    dbErr := errors.New("pool exhausted")

    mock.ExpectBegin().WillReturnError(dbErr)

    client := newTestClient(t, mock)
    tx, err := client.Begin(context.Background())

    if err == nil {
        t.Fatal("expected an error, got nil")
    }
    if tx != nil {
        t.Error("expected nil tx on error")
    }
    if !errors.Is(err, dbErr) {
        t.Errorf("expected error to wrap %v, got %v", dbErr, err)
    }

    if err := mock.ExpectationsWereMet(); err != nil {
        t.Errorf("unfulfilled expectations: %v", err)
    }
}

// ── Ping ──────────────────────────────────────────────────────────────────────

func TestClient_Ping_Success(t *testing.T) {
    mock := newMockPool(t)

    mock.ExpectPing()

    client := newTestClient(t, mock)
    if err := client.Ping(context.Background()); err != nil {
        t.Fatalf("expected no error, got %v", err)
    }

    if err := mock.ExpectationsWereMet(); err != nil {
        t.Errorf("unfulfilled expectations: %v", err)
    }
}

func TestClient_Ping_Error(t *testing.T) {
    mock := newMockPool(t)
    dbErr := errors.New("connection refused")

    mock.ExpectPing().WillReturnError(dbErr)

    client := newTestClient(t, mock)
    err := client.Ping(context.Background())

    if err == nil {
        t.Fatal("expected an error, got nil")
    }
    if !errors.Is(err, dbErr) {
        t.Errorf("expected error to wrap %v, got %v", dbErr, err)
    }

    if err := mock.ExpectationsWereMet(); err != nil {
        t.Errorf("unfulfilled expectations: %v", err)
    }
}

// ── Close ─────────────────────────────────────────────────────────────────────

func TestClient_Close(t *testing.T) {
    mock, err := pgxmock.NewPool()
    if err != nil {
        t.Fatalf("failed to create pgxmock pool: %v", err)
    }

    mock.ExpectClose()

    client := newTestClient(t, mock)
    client.Close() // must not panic and must call pool.Close exactly once

    if err := mock.ExpectationsWereMet(); err != nil {
        t.Errorf("unfulfilled expectations: %v", err)
    }
}

// ── NoopClient ────────────────────────────────────────────────────────────────

func TestNoopClient_Builder(t *testing.T) {
    c := NewNoopClient(testutil.TestLogger(t))
    b := c.Builder()

    sql, args, err := b.Select("id").From("users").Where("id = ?", 1).ToSql()
    if err != nil {
        t.Fatalf("ToSql failed: %v", err)
    }
    if sql != "SELECT id FROM users WHERE id = $1" {
        t.Errorf("unexpected SQL: %q", sql)
    }
    if len(args) != 1 || args[0] != 1 {
        t.Errorf("unexpected args: %v", args)
    }
}

func TestNoopClient_Query(t *testing.T) {
    c := NewNoopClient(testutil.TestLogger(t))
    rows, err := c.Query(context.Background(), "SELECT 1")

    if err == nil {
        t.Fatal("expected an error from noop Query")
    }
    if rows != nil {
        t.Error("expected nil rows from noop Query")
    }
}

func TestNoopClient_QueryRow(t *testing.T) {
    c := NewNoopClient(testutil.TestLogger(t))
    row := c.QueryRow(context.Background(), "SELECT 1")

    if row == nil {
        t.Fatal("expected non-nil row from noop QueryRow")
    }
    // Scan must return an error
    if err := row.Scan(); err == nil {
        t.Error("expected Scan on noop row to return an error")
    }
}

func TestNoopClient_Exec(t *testing.T) {
    c := NewNoopClient(testutil.TestLogger(t))
    _, err := c.Exec(context.Background(), "INSERT INTO t VALUES (1)")

    if err == nil {
        t.Fatal("expected an error from noop Exec")
    }
}

func TestNoopClient_Begin(t *testing.T) {
    c := NewNoopClient(testutil.TestLogger(t))
    tx, err := c.Begin(context.Background())

    if err == nil {
        t.Fatal("expected an error from noop Begin")
    }
    if tx != nil {
        t.Error("expected nil tx from noop Begin")
    }
}

func TestNoopClient_Ping(t *testing.T) {
    c := NewNoopClient(testutil.TestLogger(t))
    if err := c.Ping(context.Background()); err != nil {
        t.Fatalf("expected noop Ping to succeed, got %v", err)
    }
}

func TestNoopClient_Close(t *testing.T) {
    c := NewNoopClient(testutil.TestLogger(t))
    c.Close() // must not panic
}

func TestNoopClient_ImplementsDBClientInterface(t *testing.T) {
    var _ DBClientInterface = NewNoopClient(testutil.TestLogger(t))
}
