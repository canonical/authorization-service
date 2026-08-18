// Copyright 2026 Canonical Ltd.
// SPDX-License-Identifier: AGPL-3.0-only

package rest

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"

	"github.com/grpc-ecosystem/grpc-gateway/v2/runtime"
	"github.com/prometheus/client_golang/prometheus"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	"github.com/canonical/authorization-service/internal/metrics"
)

// ServerConfig represents server configuration
type ServerConfig interface {
	GetHTTPAddress() string
}

// Gateway represents the REST API gateway
type Gateway struct {
	server *http.Server
	mux    *runtime.ServeMux
	logger *slog.Logger
}

// NewGateway creates a new REST API gateway. reg registers the request-count
// and latency collectors; pass a fresh internal/metrics.NewRegistry() per process.
func NewGateway(cfg ServerConfig, reg *prometheus.Registry, logger *slog.Logger) (*Gateway, error) {
	mux := runtime.NewServeMux(
		runtime.WithHealthEndpointAt(nil, "/healthz"),
	)

	gateway := &Gateway{
		mux:    mux,
		logger: logger,
	}

	restMetrics := metrics.NewRESTMetrics(reg)
	server := &http.Server{
		Addr:    cfg.GetHTTPAddress(),
		Handler: restMetrics.Middleware(loggingMiddleware(mux, logger)),
	}

	gateway.server = server

	return gateway, nil
}

// Start starts the REST gateway and connects to the gRPC server
func (g *Gateway) Start(grpcAddress string) error {
	// Connect to gRPC server
	conn, err := grpc.NewClient(
		grpcAddress,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		return fmt.Errorf("failed to connect to gRPC server: %w", err)
	}
	defer conn.Close()

	// TODO: Register gRPC gateway handlers when proto is generated
	// err = authzv1.RegisterPermissionsServiceHandler(context.Background(), g.mux, conn)
	// if err != nil {
	//     return fmt.Errorf("failed to register permissions service handler: %w", err)
	// }

	g.logger.Info("REST gateway listening", "address", g.server.Addr)
	return g.server.ListenAndServe()
}

// Shutdown gracefully shuts down the gateway
func (g *Gateway) Shutdown(ctx context.Context) error {
	g.logger.Info("Shutting down REST gateway")
	return g.server.Shutdown(ctx)
}

// loggingMiddleware logs HTTP requests
func loggingMiddleware(next http.Handler, logger *slog.Logger) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		logger.Info("HTTP request",
			"method", r.Method,
			"path", r.URL.Path,
			"remote", r.RemoteAddr,
		)
		next.ServeHTTP(w, r)
	})
}
