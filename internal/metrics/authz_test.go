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

	m.RecordCheck("allow", "ok", "cookie", 150*time.Millisecond)

	if got := testutil.ToFloat64(m.checksTotal.WithLabelValues("allow", "ok", "cookie")); got != 1 {
		t.Errorf("checksTotal{allow,ok,cookie} = %v, want 1", got)
	}

	snap := histogramSnapshot(t, m.checkDuration.WithLabelValues("allow", "cookie"))
	if snap.GetSampleCount() != 1 {
		t.Errorf("checkDuration{allow,cookie} sample count = %d, want 1", snap.GetSampleCount())
	}
	if got, want := snap.GetSampleSum(), 0.15; got < want-0.01 || got > want+0.01 {
		t.Errorf("checkDuration{allow,cookie} sample sum = %v, want ~%v", got, want)
	}
}

func TestCheckRecorder_RecordCheck_DistinctLabels(t *testing.T) {
	reg := prometheus.NewRegistry()
	m := NewCheckRecorder(reg)

	m.RecordCheck("deny", "no_cookie", "none", time.Millisecond)
	m.RecordCheck("deny", "conflicting_credentials", "none", time.Millisecond)
	m.RecordCheck("allow", "ok", "client_credentials", time.Millisecond)

	if got := testutil.ToFloat64(m.checksTotal.WithLabelValues("deny", "no_cookie", "none")); got != 1 {
		t.Errorf("checksTotal{deny,no_cookie,none} = %v, want 1", got)
	}
	if got := testutil.ToFloat64(m.checksTotal.WithLabelValues("deny", "conflicting_credentials", "none")); got != 1 {
		t.Errorf("checksTotal{deny,conflicting_credentials,none} = %v, want 1", got)
	}
	if got := testutil.ToFloat64(m.checksTotal.WithLabelValues("allow", "ok", "client_credentials")); got != 1 {
		t.Errorf("checksTotal{allow,ok,client_credentials} = %v, want 1", got)
	}
}

func TestCheckRecorder_ObserveHydraVerify(t *testing.T) {
	reg := prometheus.NewRegistry()
	m := NewCheckRecorder(reg)

	m.ObserveHydraVerify(12 * time.Millisecond)

	snap := histogramSnapshot(t, m.hydraVerifyTime)
	if snap.GetSampleCount() != 1 {
		t.Errorf("hydraVerifyTime sample count = %d, want 1", snap.GetSampleCount())
	}
}

func TestCheckRecorder_ObserveSTSExchange(t *testing.T) {
	reg := prometheus.NewRegistry()
	m := NewCheckRecorder(reg)

	m.ObserveSTSExchange("session", 10*time.Millisecond)
	m.ObserveSTSExchange("token", 20*time.Millisecond)

	snapSession := histogramSnapshot(t, m.stsExchangeTime.WithLabelValues("session"))
	if snapSession.GetSampleCount() != 1 {
		t.Errorf("stsExchangeTime{session} sample count = %d, want 1", snapSession.GetSampleCount())
	}

	snapToken := histogramSnapshot(t, m.stsExchangeTime.WithLabelValues("token"))
	if snapToken.GetSampleCount() != 1 {
		t.Errorf("stsExchangeTime{token} sample count = %d, want 1", snapToken.GetSampleCount())
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
