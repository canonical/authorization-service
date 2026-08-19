// Copyright 2026 Canonical Ltd.
// SPDX-License-Identifier: AGPL-3.0-only

// Package metrics is the only package that imports prometheus/client_golang.
// Service and repository packages define small Metrics interfaces (with a
// NoopMetrics default) satisfied structurally by the recorders in this
// package, so business logic never depends on Prometheus directly.
package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
)

// NewRegistry creates a fresh Prometheus registry seeded with the standard Go
// runtime and process collectors. A custom registry (rather than the global
// DefaultRegisterer) is used so that constructing recorders more than once
// within a single test binary never panics on duplicate registration.
func NewRegistry() *prometheus.Registry {
	reg := prometheus.NewRegistry()
	reg.MustRegister(
		collectors.NewGoCollector(),
		collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}),
	)
	return reg
}
