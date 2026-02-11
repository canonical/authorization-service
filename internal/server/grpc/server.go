package grpc

import (
    "fmt"
    "log/slog"
    "net"

    "google.golang.org/grpc"
    "google.golang.org/grpc/health"
    "google.golang.org/grpc/health/grpc_health_v1"
    "google.golang.org/grpc/reflection"

    "github.com/canonical/authorization-service/internal/config"
    "github.com/canonical/authorization-service/internal/service/authz"
    "github.com/canonical/authorization-service/internal/service/permissions"
)

// Server represents the gRPC server
type Server struct {
    grpcServer         *grpc.Server
    listener           net.Listener
    permissionsService *permissions.Service
    authzService       *authz.Service
    externalAuthz      *authz.ExternalAuthzService

    logger *slog.Logger
}

// ServerOption is a functional option for configuring the Server
type ServerOption func(*Server)

// WithPermissionsService sets the permissions service
func WithPermissionsService(svc *permissions.Service) ServerOption {
    return func(s *Server) {
        s.permissionsService = svc
    }
}

// WithAuthzService sets the authorization service
func WithAuthzService(svc *authz.Service) ServerOption {
    return func(s *Server) {
        s.authzService = svc
    }
}

// WithExternalAuthz sets the external authorization service
func WithExternalAuthz(svc *authz.ExternalAuthzService) ServerOption {
    return func(s *Server) {
        s.externalAuthz = svc
    }
}

// NewServer creates a new gRPC server
func NewServer(cfg *config.ServerConfig, logger *slog.Logger, opts ...ServerOption) (*Server, error) {
    listener, err := net.Listen("tcp", cfg.GetGRPCAddress())
    if err != nil {
        return nil, fmt.Errorf("failed to create listener: %w", err)
    }

    grpcServer := grpc.NewServer(
        grpc.ChainUnaryInterceptor(
            loggingInterceptor(logger),
            recoveryInterceptor(logger),
        ),
    )

    server := &Server{
        grpcServer: grpcServer,
        listener:   listener,
        logger:     logger,
    }

    for _, opt := range opts {
        opt(server)
    }

    // Health check service
    healthServer := health.NewServer()
    grpc_health_v1.RegisterHealthServer(grpcServer, healthServer)
    healthServer.SetServingStatus("", grpc_health_v1.HealthCheckResponse_SERVING)

    if cfg.Development {
        reflection.Register(grpcServer)
    }

    if server.externalAuthz != nil {
        server.externalAuthz.Register(grpcServer)
        logger.Info("Registered Envoy External Authorization service")
    }

    return server, nil
}

// Start starts the gRPC server
func (s *Server) Start() error {
    s.logger.Info("gRPC server listening", "address", s.listener.Addr().String())
    return s.grpcServer.Serve(s.listener)
}

// GracefulStop gracefully stops the server
func (s *Server) GracefulStop() {
    s.logger.Info("Gracefully stopping gRPC server")
    s.grpcServer.GracefulStop()
}

// Stop immediately stops the server
func (s *Server) Stop() {
    s.logger.Info("Stopping gRPC server")
    s.grpcServer.Stop()
}
