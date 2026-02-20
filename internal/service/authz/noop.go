package authz

import (
    "context"
    "log/slog"

    envoyAuth "github.com/envoyproxy/go-control-plane/envoy/service/auth/v3"
    "google.golang.org/genproto/googleapis/rpc/status"
    "google.golang.org/grpc"
    "google.golang.org/grpc/codes"
)

// Compile-time check to ensure NoopExternalAuthzService implements ExternalAuthzServiceInterface
var _ ExternalAuthzServiceInterface = (*NoopExternalAuthzService)(nil)

// NoopExternalAuthzService is a no-operation implementation of ExternalAuthzServiceInterface
// that always allows requests without performing any authorization checks.
type NoopExternalAuthzService struct {
    envoyAuth.UnimplementedAuthorizationServer
    logger *slog.Logger
}

// NewNoopExternalAuthzService creates a new noop external authorization service
func NewNoopExternalAuthzService(logger *slog.Logger) *NoopExternalAuthzService {
    return &NoopExternalAuthzService{
        logger: logger,
    }
}

// Register registers the noop service with the gRPC server
func (s *NoopExternalAuthzService) Register(grpcServer *grpc.Server) {
    envoyAuth.RegisterAuthorizationServer(grpcServer, s)
}

// Check always returns OK, allowing all requests without any authorization checks
func (s *NoopExternalAuthzService) Check(ctx context.Context, req *envoyAuth.CheckRequest) (*envoyAuth.CheckResponse, error) {
    s.logger.Debug("Noop authorization check - allowing request")

    return &envoyAuth.CheckResponse{
        Status: &status.Status{
            Code: int32(codes.OK),
        },
        HttpResponse: &envoyAuth.CheckResponse_OkResponse{
            OkResponse: &envoyAuth.OkHttpResponse{},
        },
    }, nil
}

// Compile-time check to ensure NoopService implements ServiceInterface
var _ ServiceInterface = (*NoopService)(nil)

// NoopService is a no-operation implementation of ServiceInterface
// that always allows requests without performing any authorization checks.
type NoopService struct {
    logger *slog.Logger
}

// NewNoopService creates a new noop authorization service
func NewNoopService(logger *slog.Logger) *NoopService {
    return &NoopService{
        logger: logger,
    }
}

// Check always returns allowed without performing any authorization checks
func (s *NoopService) Check(ctx context.Context, user, resource, action string) (*CheckResponse, error) {
    s.logger.Debug("Noop authorization check - allowing request",
        "user", user,
        "resource", resource,
        "action", action,
    )

    return &CheckResponse{
        Allowed: true,
        Reason:  "noop",
    }, nil
}

// GrantAccess is a no-op that always succeeds
func (s *NoopService) GrantAccess(ctx context.Context, user, resource, action string) error {
    s.logger.Debug("Noop grant access - no action taken",
        "user", user,
        "resource", resource,
        "action", action,
    )
    return nil
}

// RevokeAccess is a no-op that always succeeds
func (s *NoopService) RevokeAccess(ctx context.Context, user, resource, action string) error {
    s.logger.Debug("Noop revoke access - no action taken",
        "user", user,
        "resource", resource,
        "action", action,
    )
    return nil
}
