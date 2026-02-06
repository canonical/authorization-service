package openfga

import (
	"context"
	"log/slog"
)

// NoopClient implements the Client interface as a no-op for development/testing
type NoopClient struct {
	logger *slog.Logger
}

// NewNoopClient creates a new no-op OpenFGA client
func NewNoopClient(logger *slog.Logger) *NoopClient {
	return &NoopClient{
		logger: logger,
	}
}

// Check performs a no-op authorization check (always returns allowed)
func (c *NoopClient) Check(ctx context.Context, req *CheckRequest) (*CheckResponse, error) {
	c.logger.Debug("NoopClient: Check called", "user", req.User, "relation", req.Relation, "object", req.Object)
	return &CheckResponse{Allowed: true}, nil
}

// Write performs a no-op write
func (c *NoopClient) Write(ctx context.Context, req *WriteRequest) (*WriteResponse, error) {
	c.logger.Debug("NoopClient: Write called", "writes", len(req.Writes), "deletes", len(req.Deletes))
	return &WriteResponse{Success: true}, nil
}

// Read performs a no-op read
func (c *NoopClient) Read(ctx context.Context, req *ReadRequest) (*ReadResponse, error) {
	c.logger.Debug("NoopClient: Read called", "user", req.User, "relation", req.Relation, "object", req.Object)
	return &ReadResponse{Tuples: []Tuple{}}, nil
}

// Close is a no-op
func (c *NoopClient) Close() error {
	return nil
}
