// Copyright 2026 Canonical Ltd.
// SPDX-License-Identifier: AGPL-3.0-only

package metrics

import (
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
)

func TestCheckRecorder_RecordCheck(t *testing.T) {
	reg := prometheus.NewRegistry()
	m := NewCheckRecorder(reg)

	m.RecordCheck("allow", "ok", 150*time.Millisecond)

	if got := testutil.ToFloat64(m.checksTotal.WithLabelValues("allow", "ok")); got != 1 {
		t.Errorf("checksTotal{allow,ok} = %v, want 1", got)
	}

	snap := histogramSnapshot(t, m.checkDuration.WithLabelValues("allow"))
	if snap.GetSampleCount() != 1 {
		t.Errorf("checkDuration{allow} sample count = %d, want 1", snap.GetSampleCount())
	}
	if got, want := snap.GetSampleSum(), 0.15; got < want-0.01 || got > want+0.01 {
		t.Errorf("checkDuration{allow} sample sum = %v, want ~%v", got, want)
	}
}

func TestCheckRecorder_RecordCheck_DistinctLabels(t *testing.T) {
	reg := prometheus.NewRegistry()
	m := NewCheckRecorder(reg)

	m.RecordCheck("deny", "no_cookie", time.Millisecond)
	m.RecordCheck("deny", "no_session", time.Millisecond)

	if got := testutil.ToFloat64(m.checksTotal.WithLabelValues("deny", "no_cookie")); got != 1 {
		t.Errorf("checksTotal{deny,no_cookie} = %v, want 1", got)
	}
	if got := testutil.ToFloat64(m.checksTotal.WithLabelValues("deny", "no_session")); got != 1 {
		t.Errorf("checksTotal{deny,no_session} = %v, want 1", got)
	}
}

func TestCheckRecorder_ObserveSTSExchange(t *testing.T) {
	reg := prometheus.NewRegistry()
	m := NewCheckRecorder(reg)

	m.ObserveSTSExchange(10 * time.Millisecond)
	m.ObserveSTSExchange(20 * time.Millisecond)

	snap := histogramSnapshot(t, m.stsExchangeTime)
	if snap.GetSampleCount() != 2 {
		t.Errorf("stsExchangeTime sample count = %d, want 2", snap.GetSampleCount())
	}
}

func TestCheckRecorder_ObserveResourceMap(t *testing.T) {
	reg := prometheus.NewRegistry()
	m := NewCheckRecorder(reg)

	m.ObserveResourceMap(5 * time.Millisecond)

	snap := histogramSnapshot(t, m.resourceMapTime)
	if snap.GetSampleCount() != 1 {
		t.Errorf("resourceMapTime sample count = %d, want 1", snap.GetSampleCount())
	}
}

func TestCheckRecorder_ObserveOpenFGACheck(t *testing.T) {
	reg := prometheus.NewRegistry()
	m := NewCheckRecorder(reg)

	m.ObserveOpenFGACheck(30 * time.Millisecond)

	snap := histogramSnapshot(t, m.openFGACheckTime)
	if snap.GetSampleCount() != 1 {
		t.Errorf("openFGACheckTime sample count = %d, want 1", snap.GetSampleCount())
	}
}
