package openfga

import (
	"context"
)

// Client defines the interface for OpenFGA operations
type Client interface {
	// Check performs an authorization check
	Check(ctx context.Context, req *CheckRequest) (*CheckResponse, error)
	// Write writes authorization tuples
	Write(ctx context.Context, req *WriteRequest) (*WriteResponse, error)
	// Read reads authorization tuples
	Read(ctx context.Context, req *ReadRequest) (*ReadResponse, error)
	// Close closes the client connection
	Close() error
}

// CheckRequest represents an authorization check request
type CheckRequest struct {
	User     string
	Relation string
	Object   string
}

// CheckResponse represents an authorization check response
type CheckResponse struct {
	Allowed bool
}

// WriteRequest represents a write request for authorization tuples
type WriteRequest struct {
	Writes  []Tuple
	Deletes []Tuple
}

// WriteResponse represents a write response
type WriteResponse struct {
	Success bool
}

// ReadRequest represents a read request for authorization tuples
type ReadRequest struct {
	User     string
	Relation string
	Object   string
}

// ReadResponse represents a read response
type ReadResponse struct {
	Tuples []Tuple
}

// Tuple represents an authorization tuple
type Tuple struct {
	User     string
	Relation string
	Object   string
}
