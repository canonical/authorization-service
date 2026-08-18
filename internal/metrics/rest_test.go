// Copyright 2026 Canonical Ltd.
// SPDX-License-Identifier: AGPL-3.0-only

package metrics

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
)

func TestRESTMetrics_Middleware(t *testing.T) {
	tests := []struct {
		name       string
		handler    http.HandlerFunc
		wantStatus string
	}{
		{
			name:       "explicit status",
			handler:    func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusNotFound) },
			wantStatus: "404",
		},
		{
			name:       "default status",
			handler:    func(w http.ResponseWriter, r *http.Request) {},
			wantStatus: "200",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			reg := prometheus.NewRegistry()
			m := NewRESTMetrics(reg)
			wrapped := m.Middleware(tc.handler)

			req := httptest.NewRequest(http.MethodGet, "/anything", nil)
			rec := httptest.NewRecorder()
			wrapped.ServeHTTP(rec, req)

			if got := testutil.ToFloat64(m.requestsTotal.WithLabelValues(http.MethodGet, tc.wantStatus)); got != 1 {
				t.Errorf("requestsTotal{GET,%s} = %v, want 1", tc.wantStatus, got)
			}

			snap := histogramSnapshot(t, m.duration.WithLabelValues(http.MethodGet))
			if snap.GetSampleCount() != 1 {
				t.Errorf("duration{GET} sample count = %d, want 1", snap.GetSampleCount())
			}
		})
	}
}
