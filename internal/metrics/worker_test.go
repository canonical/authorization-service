// Copyright 2026 Canonical Ltd.
// SPDX-License-Identifier: AGPL-3.0-only

package metrics

import (
	"math"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
)

func TestProcessorRecorder(t *testing.T) {
	reg := prometheus.NewRegistry()
	m := NewProcessorRecorder(reg)

	m.IncProcessed("dummy")
	m.IncRetry("dummy")
	m.IncPermanentFailure("dummy", "msg-id-ignored", "decode_failed")

	if got := testutil.ToFloat64(m.processedTotal.WithLabelValues("dummy")); got != 1 {
		t.Errorf("processedTotal{dummy} = %v, want 1", got)
	}
	if got := testutil.ToFloat64(m.retriedTotal.WithLabelValues("dummy")); got != 1 {
		t.Errorf("retriedTotal{dummy} = %v, want 1", got)
	}
	if got := testutil.ToFloat64(m.failuresTotal.WithLabelValues("dummy", "decode_failed")); got != 1 {
		t.Errorf("failuresTotal{dummy,decode_failed} = %v, want 1", got)
	}
}

func TestWorkerLoopRecorder(t *testing.T) {
	reg := prometheus.NewRegistry()
	m := NewWorkerLoopRecorder(reg)

	m.ObserveBatchClaimed(25)
	m.ObserveRowDuration(50 * time.Millisecond)

	batchSnap := histogramSnapshot(t, m.batchClaimed)
	if batchSnap.GetSampleCount() != 1 {
		t.Errorf("batchClaimed sample count = %d, want 1", batchSnap.GetSampleCount())
	}
	if got, want := batchSnap.GetSampleSum(), 25.0; got != want {
		t.Errorf("batchClaimed sample sum = %v, want %v", got, want)
	}

	rowSnap := histogramSnapshot(t, m.rowDuration)
	if rowSnap.GetSampleCount() != 1 {
		t.Errorf("rowDuration sample count = %d, want 1", rowSnap.GetSampleCount())
	}
}

func TestReaperRecorder_ObserveReclaim(t *testing.T) {
	reg := prometheus.NewRegistry()
	m := NewReaperRecorder(reg)

	m.ObserveReclaim(3, 10*time.Millisecond)
	m.ObserveReclaim(2, 20*time.Millisecond)

	if got := testutil.ToFloat64(m.reclaimedTotal); got != 5 {
		t.Errorf("reclaimedTotal = %v, want 5", got)
	}

	snap := histogramSnapshot(t, m.reclaimDuration)
	if snap.GetSampleCount() != 2 {
		t.Errorf("reclaimDuration sample count = %d, want 2", snap.GetSampleCount())
	}
}

func TestReaperRecorder_SetLastRunTimestamp(t *testing.T) {
	reg := prometheus.NewRegistry()
	m := NewReaperRecorder(reg)

	before := time.Now().Unix()
	m.SetLastRunTimestamp()
	after := time.Now().Unix()

	got := testutil.ToFloat64(m.lastRunSeconds)
	if got < float64(before) || got > float64(after) {
		t.Errorf("lastRunSeconds = %v, want within [%d, %d]", got, before, after)
	}
	if math.IsNaN(got) {
		t.Error("lastRunSeconds is NaN")
	}
}
