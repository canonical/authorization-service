// Copyright 2026 Canonical Ltd.
// SPDX-License-Identifier: AGPL-3.0-only

package metrics

import (
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
)

// histogramSnapshot writes o (a concrete prometheus.Histogram, e.g. returned
// by HistogramVec.WithLabelValues or a plain Histogram) into a dto.Metric and
// returns its Histogram field, giving access to SampleCount/SampleSum for
// assertions that testutil.ToFloat64 cannot make (it only supports
// single-value collectors).
func histogramSnapshot(t *testing.T, o prometheus.Observer) *dto.Histogram {
	t.Helper()

	h, ok := o.(prometheus.Histogram)
	if !ok {
		t.Fatalf("observer %T does not implement prometheus.Histogram", o)
	}

	var m dto.Metric
	if err := h.Write(&m); err != nil {
		t.Fatalf("failed to write histogram metric: %v", err)
	}
	return m.GetHistogram()
}
