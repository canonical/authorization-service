package grpc

import (
	"fmt"
	"log/slog"
	"net"

	authv3 "github.com/envoyproxy/go-control-plane/envoy/service/auth/v3"
	"google.golang.org/grpc"
	"google.golang.org/grpc/health"
	"google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/reflection"
)

// ServerConfig represents server configuration
type ServerConfig interface {
	GetGRPCAddress() string
}

// Server represents the gRPC server
type Server struct {
	grpcServer *grpc.Server
	listener   net.Listener
	logger     *slog.Logger
}

// Services holds all gRPC service implementations
type Services interface {
	// GetEnvoyAuthz returns the Envoy authorization service
	GetEnvoyAuthz() AuthorizationService
}

// AuthorizationService defines the authorization service interface
type AuthorizationService interface {
	// Embed the official Envoy authorization server interface
	authv3.AuthorizationServer
}

// NewServer creates a new gRPC server
func NewServer(cfg ServerConfig, services Services, logger *slog.Logger) (*Server, error) {
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

	// Register health check service
	healthServer := health.NewServer()
	grpc_health_v1.RegisterHealthServer(grpcServer, healthServer)
	healthServer.SetServingStatus("", grpc_health_v1.HealthCheckResponse_SERVING)

	// Register reflection service for development
	reflection.Register(grpcServer)

	// Register authorization services if provided
	if services != nil {
		if authzSvc := services.GetEnvoyAuthz(); authzSvc != nil {
			logger.Info("Registering Envoy authorization service")
			authv3.RegisterAuthorizationServer(grpcServer, authzSvc)
		}
	}

	return &Server{
		grpcServer: grpcServer,
		listener:   listener,
		logger:     logger,
	}, nil
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
