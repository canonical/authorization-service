package grpc

import (
	"fmt"
	"log/slog"
	"net"

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
	// Add service interfaces here as they are implemented
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

	// TODO: Register application services when proto is generated
	// authzv1.RegisterPermissionsServiceServer(grpcServer, permissionsServer)
	// authzv1.RegisterAuthorizationServer(grpcServer, authzServer)

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
