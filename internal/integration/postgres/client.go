// Copyright 2026 Canonical Ltd.
// SPDX-License-Identifier: AGPL-3.0-only

package postgres

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	sq "github.com/Masterminds/squirrel"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

const (
	defaultPage     uint64 = 1
	defaultPageSize uint64 = 100
)

// DBClientInterface defines the interface for database operations using pgx native types.
type DBClientInterface interface {
	// Query executes a query that returns rows
	Query(ctx context.Context, query string, args ...interface{}) (pgx.Rows, error)
	// QueryRow executes a query that returns at most one row
	QueryRow(ctx context.Context, query string, args ...interface{}) pgx.Row
	// Exec executes a query without returning any rows
	Exec(ctx context.Context, query string, args ...interface{}) (pgconn.CommandTag, error)
	// Begin starts a new transaction
	Begin(ctx context.Context) (pgx.Tx, error)
	// Builder returns a pre-configured squirrel StatementBuilderType for Postgres ($N placeholders)
	Builder() sq.StatementBuilderType
	// Ping verifies the connection to the database is still alive
	Ping(ctx context.Context) error
	// Close closes the connection pool
	Close()
}

// poolInterface defines the subset of pgxpool.Pool methods used by Client.
type poolInterface interface {
	Query(ctx context.Context, sql string, args ...interface{}) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...interface{}) pgx.Row
	Exec(ctx context.Context, sql string, args ...interface{}) (pgconn.CommandTag, error)
	Begin(ctx context.Context) (pgx.Tx, error)
	Ping(ctx context.Context) error
	Close()
}

// builderWithDollar returns a squirrel StatementBuilderType with Dollar placeholders.
func builderWithDollar() sq.StatementBuilderType {
	return sq.StatementBuilder.PlaceholderFormat(sq.Dollar)
}

// Compile-time check to ensure Client implements DBClientInterface
var _ DBClientInterface = (*Client)(nil)

// Client wraps a pgxpool.Pool for PostgreSQL access.
type Client struct {
	pool    poolInterface
	builder sq.StatementBuilderType

	logger *slog.Logger
	tracer trace.Tracer
}

// Config holds PostgreSQL client configuration
type Config struct {
	Host            string
	Port            int
	User            string
	Password        string
	DBName          string
	SSLMode         string
	MaxOpenConns    int32
	MaxIdleConns    int32
	ConnMaxLifetime time.Duration
	ConnMaxIdleTime time.Duration
	ConnectTimeout  time.Duration
}

// DSN builds the PostgreSQL connection string (Data Source Name)
func (c *Config) DSN() string {
	return fmt.Sprintf(
		"host=%s port=%d user=%s password=%s dbname=%s sslmode=%s connect_timeout=%d",
		c.Host,
		c.Port,
		c.User,
		c.Password,
		c.DBName,
		c.SSLMode,
		int(c.ConnectTimeout.Seconds()),
	)
}

// NewClient creates a new PostgreSQL client backed by a pgxpool.Pool.
func NewClient(cfg Config, logger *slog.Logger, tracer trace.Tracer) (*Client, error) {
	poolCfg, err := pgxpool.ParseConfig(cfg.DSN())
	if err != nil {
		return nil, fmt.Errorf("failed to parse postgres config: %w", err)
	}

	if cfg.MaxOpenConns > 0 {
		poolCfg.MaxConns = cfg.MaxOpenConns
	}
	if cfg.MaxIdleConns > 0 {
		poolCfg.MinConns = cfg.MaxIdleConns
	}
	if cfg.ConnMaxLifetime > 0 {
		poolCfg.MaxConnLifetime = cfg.ConnMaxLifetime
	}
	if cfg.ConnMaxIdleTime > 0 {
		poolCfg.MaxConnIdleTime = cfg.ConnMaxIdleTime
	}

	connectTimeout := 10 * time.Second
	if cfg.ConnectTimeout == 0 {
		connectTimeout = cfg.ConnectTimeout
	}
	connectCtx, cancel := context.WithTimeout(context.Background(), connectTimeout)
	defer cancel()

	pool, err := pgxpool.NewWithConfig(connectCtx, poolCfg)
	if err != nil {
		return nil, fmt.Errorf("failed to create pgx pool: %w", err)
	}

	// Verify at least one connection can be established
	if err := pool.Ping(connectCtx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("failed to ping postgres: %w", err)
	}

	logger.Info("PostgreSQL client connected",
		"host", cfg.Host,
		"port", cfg.Port,
		"dbname", cfg.DBName,
		"sslmode", cfg.SSLMode,
		"max_conns", poolCfg.MaxConns,
		"min_conns", poolCfg.MinConns,
	)

	return &Client{
		pool:    pool,
		builder: builderWithDollar(),
		tracer:  tracer,
		logger:  logger,
	}, nil
}

func Offset(pageParam int64, pageSize uint64) uint64 {
	if pageParam <= 0 {
		return (defaultPage - 1) * pageSize
	}
	return uint64(pageParam-1) * pageSize
}

func PageSize(sizeParam int64) uint64 {
	if sizeParam <= 0 {
		return defaultPageSize
	}
	return uint64(sizeParam)
}

// Builder returns a pre-configured squirrel StatementBuilderType for Postgres ($ placeholders)
func (c *Client) Builder() sq.StatementBuilderType {
	return c.builder
}

// Query executes a query that returns rows
func (c *Client) Query(ctx context.Context, query string, args ...interface{}) (pgx.Rows, error) {
	ctx, span := c.tracer.Start(ctx, "integration.postgres.Query",
		trace.WithAttributes(attribute.String("db.statement", query)),
	)
	defer span.End()

	start := time.Now()

	rows, err := c.pool.Query(ctx, query, args...)
	duration := time.Since(start)

	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		c.logger.Error("Failed to execute query", "query", query, "error", err, "duration_ms", duration.Milliseconds())
		return nil, fmt.Errorf("failed to execute query: %w", err)
	}

	span.SetStatus(codes.Ok, "")
	c.logger.Debug("Query executed", "query", query, "duration_ms", duration.Milliseconds())
	return rows, nil
}

// QueryRow executes a query that is expected to return at most one row
func (c *Client) QueryRow(ctx context.Context, query string, args ...interface{}) pgx.Row {
	ctx, span := c.tracer.Start(ctx, "integration.postgres.QueryRow",
		trace.WithAttributes(attribute.String("db.statement", query)),
	)
	defer span.End()

	c.logger.Debug("QueryRow executed", "query", query)
	span.SetStatus(codes.Ok, "")
	return c.pool.QueryRow(ctx, query, args...)
}

// Exec executes a query without returning any rows
func (c *Client) Exec(ctx context.Context, query string, args ...interface{}) (pgconn.CommandTag, error) {
	ctx, span := c.tracer.Start(ctx, "integration.postgres.Exec",
		trace.WithAttributes(attribute.String("db.statement", query)),
	)
	defer span.End()

	start := time.Now()

	tag, err := c.pool.Exec(ctx, query, args...)
	duration := time.Since(start)

	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		c.logger.Error("Failed to execute statement", "query", query, "error", err, "duration_ms", duration.Milliseconds())
		return pgconn.CommandTag{}, fmt.Errorf("failed to execute statement: %w", err)
	}

	span.SetAttributes(attribute.Int64("db.rows_affected", tag.RowsAffected()))
	span.SetStatus(codes.Ok, "")
	c.logger.Debug("Exec executed", "query", query, "rows_affected", tag.RowsAffected(), "duration_ms", duration.Milliseconds())
	return tag, nil
}

// Begin starts a new transaction
func (c *Client) Begin(ctx context.Context) (pgx.Tx, error) {
	tx, err := c.pool.Begin(ctx)

	if err != nil {
		return nil, fmt.Errorf("failed to begin transaction: %w", err)
	}

	return tx, nil
}

// Ping verifies a connection to the database is still alive
func (c *Client) Ping(ctx context.Context) error {
	if err := c.pool.Ping(ctx); err != nil {
		return fmt.Errorf("postgres ping failed: %w", err)
	}
	return nil
}

// Close closes the pgxpool connection pool
func (c *Client) Close() {
	c.pool.Close()
}
