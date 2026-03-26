package authz

import (
    "context"
    "fmt"
    "log/slog"

    "github.com/canonical/authorization-service/internal/integration/openfga"
)

type ServiceInterface interface {
    Check(ctx context.Context, user, resource, action string) (*CheckResponse, error)
    GrantAccess(ctx context.Context, user, resource, action string) error
    RevokeAccess(ctx context.Context, user, resource, action string) error
}

// Compile-time check to ensure Service implements ServiceInterface
var _ ServiceInterface = (*Service)(nil)

// Service handles authorization checks
type Service struct {
    fga    openfga.ClientInterface
    logger *slog.Logger
}

// NewService creates a new authorization service
func NewService(fga openfga.ClientInterface, logger *slog.Logger) *Service {
    return &Service{
        fga:    fga,
        logger: logger,
    }
}

type CheckResponse struct {
    Allowed bool   `json:"allowed"`
    Reason  string `json:"reason"`
}

// Check performs an authorization check
func (s *Service) Check(ctx context.Context, user, resource, action string) (*CheckResponse, error) {
    s.logger.Debug("Authorization check",
        "user", user,
        "resource", resource,
        "action", action,
    )

    // Perform OpenFGA check
    fgaReq := &openfga.CheckRequest{
        User:     user,
        Relation: action,
        Object:   resource,
    }

    fgaResp, err := s.fga.Check(ctx, fgaReq)
    if err != nil {
        s.logger.Error("OpenFGA check failed", "error", err)
        return nil, fmt.Errorf("authorization check failed: %w", err)
    }

    return &CheckResponse{
        Allowed: fgaResp.Allowed,
        Reason:  "evaluated",
    }, nil
}

// GrantAccess grants access to a user for a resource
func (s *Service) GrantAccess(ctx context.Context, user, resource, action string) error {
    tuple := openfga.Tuple{
        User:     user,
        Relation: action,
        Object:   resource,
    }

    req := &openfga.WriteRequest{
        Writes: []openfga.Tuple{tuple},
    }

    _, err := s.fga.Write(ctx, req)
    if err != nil {
        return fmt.Errorf("failed to grant access: %w", err)
    }

    s.logger.Info("Access granted", "user", user, "resource", resource, "action", action)
    return nil
}

// RevokeAccess revokes access from a user for a resource
func (s *Service) RevokeAccess(ctx context.Context, user, resource, action string) error {
    tuple := openfga.Tuple{
        User:     user,
        Relation: action,
        Object:   resource,
    }

    req := &openfga.WriteRequest{
        Deletes: []openfga.Tuple{tuple},
    }

    _, err := s.fga.Write(ctx, req)
    if err != nil {
        return fmt.Errorf("failed to revoke access: %w", err)
    }

    s.logger.Info("Access revoked", "user", user, "resource", resource, "action", action)
    return nil
}
