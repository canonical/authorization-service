// Copyright 2026 Canonical Ltd.
// SPDX-License-Identifier: AGPL-3.0-only

package metrics

import (
	"errors"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
)

func TestRepositoryRecorder_ObserveQuery(t *testing.T) {
	tests := []struct {
		name       string
		err        error
		wantResult string
	}{
		{name: "ok", err: nil, wantResult: "ok"},
		{name: "error", err: errors.New("boom"), wantResult: "error"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			reg := prometheus.NewRegistry()
			m := NewRepositoryRecorder(reg)

			m.ObserveQuery("find_candidates", tc.err, 5*time.Millisecond)

			if got := testutil.ToFloat64(m.queriesTotal.WithLabelValues("find_candidates", tc.wantResult)); got != 1 {
				t.Errorf("queriesTotal{find_candidates,%s} = %v, want 1", tc.wantResult, got)
			}

			snap := histogramSnapshot(t, m.duration.WithLabelValues("find_candidates"))
			if snap.GetSampleCount() != 1 {
				t.Errorf("duration{find_candidates} sample count = %d, want 1", snap.GetSampleCount())
			}
		})
	}
}
