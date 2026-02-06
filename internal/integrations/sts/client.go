package sts

import (
	"context"
	"crypto/tls"
	"fmt"
	"log/slog"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
)

// TokenClient defines the interface for token validation operations
type TokenClient interface {
	ValidateToken(ctx context.Context, token string) (*TokenInfo, error)
	Close() error
}

// Client represents the STS (Secure Token Service) gRPC client
type Client struct {
	conn   *grpc.ClientConn
	logger *slog.Logger
}

// Config holds STS client configuration
type Config struct {
	Address string
	UseTLS  bool
	Timeout time.Duration
}

// NewClient creates a new STS client
func NewClient(cfg Config, logger *slog.Logger) (*Client, error) {
	var opts []grpc.DialOption

	if cfg.UseTLS {
		tlsConfig := &tls.Config{
			MinVersion: tls.VersionTLS12,
		}
		opts = append(opts, grpc.WithTransportCredentials(credentials.NewTLS(tlsConfig)))
	} else {
		opts = append(opts, grpc.WithTransportCredentials(insecure.NewCredentials()))
	}

	ctx, cancel := context.WithTimeout(context.Background(), cfg.Timeout)
	defer cancel()

	conn, err := grpc.DialContext(ctx, cfg.Address, opts...)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to STS: %w", err)
	}

	logger.Info("STS client connected", "address", cfg.Address, "tls", cfg.UseTLS)

	return &Client{
		conn:   conn,
		logger: logger,
	}, nil
}

// ValidateToken validates a token with the STS
// This is a placeholder - actual implementation will depend on the STS proto definition
func (c *Client) ValidateToken(ctx context.Context, token string) (*TokenInfo, error) {
	// TODO: Implement when STS proto is available
	c.logger.Debug("ValidateToken called", "token_length", len(token))

	// Placeholder implementation
	return &TokenInfo{
		Valid:     true,
		Subject:   "user:example",
		ExpiresAt: time.Now().Add(time.Hour),
	}, nil
}

// TokenInfo represents information about a validated token
type TokenInfo struct {
	Valid     bool
	Subject   string
	ExpiresAt time.Time
	Claims    map[string]interface{}
}

// Close closes the STS client connection
func (c *Client) Close() error {
	if c.conn != nil {
		err := c.conn.Close()
		if err != nil {
			return fmt.Errorf("failed to close STS client: %w", err)
		}
		c.logger.Info("STS connection closed")
	}
	return nil
}
