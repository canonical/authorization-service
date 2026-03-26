package sts

import (
    "context"
    "log/slog"

    "google.golang.org/grpc"

    stsv1 "github.com/canonical/authorization-service/client/v1/sts"
)

// Compile-time check to ensure NoopClient implements SecurityTokenServiceClient
var _ stsv1.SecurityTokenServiceClient = (*NoopClient)(nil)

// NoopClient is a no-operation implementation of SecurityTokenServiceClient
// that returns mock responses without making actual RPC calls.
// Useful for testing and development without STS infrastructure.
type NoopClient struct {
    logger *slog.Logger
}

// NewNoopClient creates a new noop STS client
func NewNoopClient(logger *slog.Logger) *NoopClient {
    return &NoopClient{
        logger: logger,
    }
}

// ExchangeSession returns a mock JWT token without making actual RPC call
func (c *NoopClient) ExchangeSession(ctx context.Context, in *stsv1.ExchangeRequest, opts ...grpc.CallOption) (*stsv1.ExchangeResponse, error) {
    c.logger.Debug("Noop ExchangeSession - returning mock token",
        "session_cookie", in.GetSessionCookie(),
    )

    return &stsv1.ExchangeResponse{
        AccessToken: "mock-jwt-token",
        ExpiresIn:   3600, // 1 hour
    }, nil
}

// RevokeUserSessions returns success without actually revoking any sessions
func (c *NoopClient) RevokeUserSessions(ctx context.Context, in *stsv1.RevokeUserRequest, opts ...grpc.CallOption) (*stsv1.RevokeUserResponse, error) {
    c.logger.Debug("Noop RevokeUserSessions - returning success",
        "user_id", in.GetUserId(),
    )

    return &stsv1.RevokeUserResponse{
        Success: true,
    }, nil
}
