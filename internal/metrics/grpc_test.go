// Copyright 2026 Canonical Ltd.
// SPDX-License-Identifier: AGPL-3.0-only

package metrics

import (
	"context"
	"errors"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestGRPCMetrics_UnaryServerInterceptor(t *testing.T) {
	tests := []struct {
		name       string
		handlerErr error
		wantCode   string
	}{
		{name: "success", handlerErr: nil, wantCode: codes.OK.String()},
		{name: "grpc status error", handlerErr: status.Error(codes.NotFound, "nope"), wantCode: codes.NotFound.String()},
		{name: "plain error", handlerErr: errors.New("boom"), wantCode: codes.Unknown.String()},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			reg := prometheus.NewRegistry()
			m := NewGRPCMetrics(reg)
			interceptor := m.UnaryServerInterceptor()

			info := &grpc.UnaryServerInfo{FullMethod: "/svc.Method/Call"}
			handler := func(ctx context.Context, req interface{}) (interface{}, error) {
				return nil, tc.handlerErr
			}

			_, err := interceptor(context.Background(), nil, info, handler)
			if !errors.Is(err, tc.handlerErr) {
				t.Fatalf("interceptor returned err = %v, want %v", err, tc.handlerErr)
			}

			if got := testutil.ToFloat64(m.requestsTotal.WithLabelValues(info.FullMethod, tc.wantCode)); got != 1 {
				t.Errorf("requestsTotal{%s,%s} = %v, want 1", info.FullMethod, tc.wantCode, got)
			}

			snap := histogramSnapshot(t, m.duration.WithLabelValues(info.FullMethod))
			if snap.GetSampleCount() != 1 {
				t.Errorf("duration{%s} sample count = %d, want 1", info.FullMethod, snap.GetSampleCount())
			}
		})
	}
}
