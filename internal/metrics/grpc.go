// Copyright 2026 Canonical Ltd.
// SPDX-License-Identifier: AGPL-3.0-only

package metrics

import (
	"context"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// GRPCMetrics records request counts and latency for every unary gRPC call.
// Cardinality is bounded: method is the fixed set of registered RPCs and code
// is the fixed set of gRPC status codes.
type GRPCMetrics struct {
	requestsTotal *prometheus.CounterVec
	duration      *prometheus.HistogramVec
}

// NewGRPCMetrics registers the gRPC server collectors on reg.
func NewGRPCMetrics(reg *prometheus.Registry) *GRPCMetrics {
	m := &GRPCMetrics{
		requestsTotal: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "authz_grpc_requests_total",
			Help: "Total number of unary gRPC requests handled, by method and status code.",
		}, []string{"method", "code"}),
		duration: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Name:    "authz_grpc_request_duration_seconds",
			Help:    "Unary gRPC request handling duration in seconds, by method.",
			Buckets: prometheus.DefBuckets,
		}, []string{"method"}),
	}
	reg.MustRegister(m.requestsTotal, m.duration)
	return m
}

// UnaryServerInterceptor returns an interceptor that records request outcomes.
func (m *GRPCMetrics) UnaryServerInterceptor() grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
		start := time.Now()
		resp, err := handler(ctx, req)

		code := codes.OK
		if err != nil {
			if st, ok := status.FromError(err); ok {
				code = st.Code()
			} else {
				code = codes.Unknown
			}
		}

		m.requestsTotal.WithLabelValues(info.FullMethod, code.String()).Inc()
		m.duration.WithLabelValues(info.FullMethod).Observe(time.Since(start).Seconds())

		return resp, err
	}
}
