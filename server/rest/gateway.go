// Copyright 2026 Canonical Ltd.
// SPDX-License-Identifier: AGPL-3.0-only

package rest

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/grpc-ecosystem/grpc-gateway/v2/runtime"
	"github.com/prometheus/client_golang/prometheus"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"

	"github.com/canonical/authorization-service/internal/logging"
	"github.com/canonical/authorization-service/internal/metrics"
)

// requestIDHeader is the HTTP header carrying the request ID, forwarded from
// or generated at the REST edge, echoed back to the caller, and bridged into
// the outgoing gRPC call's metadata (see requestIDAnnotator) so both sides of
// the in-process proxy log the same ID.
const requestIDHeader = "X-Request-Id"

// requestIDMetadataKey is the gRPC metadata key requestIDAnnotator forwards
// the request ID under; must match server/grpc's requestIDInterceptor.
const requestIDMetadataKey = "x-request-id"

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
		runtime.WithMetadata(requestIDAnnotator),
	)

	gateway := &Gateway{
		mux:    mux,
		logger: logger,
	}

	restMetrics := metrics.NewRESTMetrics(reg)
	server := &http.Server{
		Addr:    cfg.GetHTTPAddress(),
		Handler: requestIDMiddleware(restMetrics.Middleware(loggingMiddleware(mux, logger))),
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

// requestIDMiddleware establishes the request ID for the request: it reads
// requestIDHeader from the incoming request (typically set by the mesh at
// the edge), generating one if absent, stores it in the context for
// downstream logging, and echoes it back as a response header. It also
// normalizes the request's own header so requestIDAnnotator can forward the
// same ID into the proxied gRPC call. It must wrap everything else, including
// metrics middleware, so every later handler can log the request ID.
func requestIDMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestID := r.Header.Get(requestIDHeader)
		if requestID == "" {
			requestID = uuid.NewString()
			r.Header.Set(requestIDHeader, requestID)
		}

		w.Header().Set(requestIDHeader, requestID)
		next.ServeHTTP(w, r.WithContext(logging.ContextWithRequestID(r.Context(), requestID)))
	})
}

// requestIDAnnotator forwards the REST request ID into the outgoing gRPC
// call's metadata, so requestIDInterceptor on the gRPC side reuses it
// instead of generating a second one for the same logical request.
func requestIDAnnotator(_ context.Context, r *http.Request) metadata.MD {
	requestID := r.Header.Get(requestIDHeader)
	if requestID == "" {
		return nil
	}
	return metadata.Pairs(requestIDMetadataKey, requestID)
}

// statusRecorder wraps an http.ResponseWriter to capture the status code
// written by the handler, for logging.
type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (r *statusRecorder) WriteHeader(code int) {
	r.status = code
	r.ResponseWriter.WriteHeader(code)
}

// loggingMiddleware logs HTTP requests
func loggingMiddleware(next http.Handler, logger *slog.Logger) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}

		next.ServeHTTP(rec, r)

		logger.InfoContext(r.Context(), "HTTP request",
			"method", r.Method,
			"path", r.URL.Path,
			"status", rec.status,
			"duration_ms", time.Since(start).Milliseconds(),
			"remote", r.RemoteAddr,
			"request_id", logging.RequestIDFromContext(r.Context()),
		)
	})
}
