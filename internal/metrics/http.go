// Copyright 2026 Canonical Ltd.
// SPDX-License-Identifier: AGPL-3.0-only

package metrics

import (
	"net/http"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// NewServer builds an HTTP server exposing reg on path, ready to Serve/Shutdown
// like any other long-running component (mirrors the gRPC/REST gateway
// lifecycle in cmd/serve.go, cmd/worker.go and cmd/listen.go).
func NewServer(addr, path string, reg *prometheus.Registry) *http.Server {
	mux := http.NewServeMux()
	mux.Handle(path, promhttp.HandlerFor(reg, promhttp.HandlerOpts{}))

	return &http.Server{
		Addr:    addr,
		Handler: mux,
	}
}
