// Copyright 2026 Canonical Ltd.
// SPDX-License-Identifier: AGPL-3.0-only

package health

import (
	"net/http"
)

// HandleHealthz writes a 200 OK JSON health check response.
func HandleHealthz(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(`{"status":"SERVING"}`))
}

// NewServer builds an HTTP server exposing GET /healthz on addr, ready to
// ListenAndServe/Shutdown like any other long-running component in cmd/worker.go
// and cmd/listen.go.
func NewServer(addr string) *http.Server {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", HandleHealthz)

	return &http.Server{
		Addr:    addr,
		Handler: mux,
	}
}
