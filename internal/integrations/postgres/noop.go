package postgres

import (
	"context"
	"fmt"
	"log/slog"

	sq "github.com/Masterminds/squirrel"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// Compile-time check to ensure NoopClient implements DBClientInterface
var _ DBClientInterface = (*NoopClient)(nil)

// NoopClient is a no-operation implementation of DBClientInterface.
// It does not connect to any real database and is intended for use in
// development environments or unit tests where a real Postgres instance
// is not available.
type NoopClient struct {
	logger  *slog.Logger
	builder sq.StatementBuilderType
}

// NewNoopClient creates a new noop Postgres client
func NewNoopClient(logger *slog.Logger) *NoopClient {
	return &NoopClient{
		logger:  logger,
		builder: sq.StatementBuilder.PlaceholderFormat(sq.Dollar),
	}
}

// Builder returns a pre-configured squirrel StatementBuilderType for Postgres
func (c *NoopClient) Builder() sq.StatementBuilderType {
	return c.builder
}

// Query is a no-op that returns an error
func (c *NoopClient) Query(_ context.Context, query string, _ ...interface{}) (pgx.Rows, error) {
	c.logger.Debug("Noop postgres Query called", "query", query)
	return nil, fmt.Errorf("noop postgres client: Query not supported")
}

// QueryRow is a no-op that returns a Row whose Scan always returns an error
func (c *NoopClient) QueryRow(_ context.Context, query string, _ ...interface{}) pgx.Row {
	c.logger.Debug("Noop postgres QueryRow called", "query", query)
	return &noopRow{}
}

// Exec is a no-op that returns an error
func (c *NoopClient) Exec(_ context.Context, query string, _ ...interface{}) (pgconn.CommandTag, error) {
	c.logger.Debug("Noop postgres Exec called", "query", query)
	return pgconn.CommandTag{}, fmt.Errorf("noop postgres client: Exec not supported")
}

// Begin is a no-op that returns an error
func (c *NoopClient) Begin(_ context.Context) (pgx.Tx, error) {
	c.logger.Debug("Noop postgres Begin called")
	return nil, fmt.Errorf("noop postgres client: Begin not supported")
}

// Ping is a no-op that always succeeds
func (c *NoopClient) Ping(_ context.Context) error {
	c.logger.Debug("Noop postgres Ping called")
	return nil
}

// Close is a no-op
func (c *NoopClient) Close() {
	c.logger.Debug("Noop postgres Close called")
}

// noopRow implements pgx.Row and always returns an error on Scan
type noopRow struct{}

func (r *noopRow) Scan(_ ...interface{}) error {
	return fmt.Errorf("noop postgres client: no rows available")
}
