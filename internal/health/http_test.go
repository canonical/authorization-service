// Copyright 2026 Canonical Ltd.
// SPDX-License-Identifier: AGPL-3.0-only

package health

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestNewServer_HealthzOK(t *testing.T) {
	srv := NewServer("127.0.0.1:0")
	if srv.Addr != "127.0.0.1:0" {
		t.Errorf("Addr = %q, want %q", srv.Addr, "127.0.0.1:0")
	}

	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	rec := httptest.NewRecorder()
	srv.Handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("GET /healthz status = %d, want %d", rec.Code, http.StatusOK)
	}
	if got := rec.Header().Get("Content-Type"); got != "application/json" {
		t.Errorf("Content-Type = %q, want %q", got, "application/json")
	}
	if got := rec.Body.String(); got != `{"status":"SERVING"}` {
		t.Errorf("response body = %q, want %q", got, `{"status":"SERVING"}`)
	}
}

func TestNewServer_MethodNotAllowed(t *testing.T) {
	srv := NewServer("127.0.0.1:0")

	for _, method := range []string{http.MethodPost, http.MethodPut, http.MethodDelete, http.MethodPatch} {
		req := httptest.NewRequest(method, "/healthz", nil)
		rec := httptest.NewRecorder()
		srv.Handler.ServeHTTP(rec, req)

		if rec.Code != http.StatusMethodNotAllowed {
			t.Errorf("%s /healthz status = %d, want %d", method, rec.Code, http.StatusMethodNotAllowed)
		}
	}
}

func TestNewServer_UnknownPath(t *testing.T) {
	srv := NewServer("127.0.0.1:0")

	for _, path := range []string{"/unknown", "/metrics", "/"} {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		rec := httptest.NewRecorder()
		srv.Handler.ServeHTTP(rec, req)

		if rec.Code != http.StatusNotFound {
			t.Errorf("GET %s status = %d, want %d", path, rec.Code, http.StatusNotFound)
		}
	}
}

func TestNewServer_ListenAndGracefulShutdown(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to listen: %v", err)
	}
	addr := ln.Addr().String()

	srv := NewServer(addr)
	errCh := make(chan error, 1)
	go func() {
		errCh <- srv.Serve(ln)
	}()

	resp, err := http.Get("http://" + addr + "/healthz")
	if err != nil {
		t.Fatalf("GET /healthz failed: %v", err)
	}
	body, err := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	if err != nil {
		t.Fatalf("reading response body failed: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /healthz status = %d, want %d", resp.StatusCode, http.StatusOK)
	}
	if got := resp.Header.Get("Content-Type"); got != "application/json" {
		t.Errorf("Content-Type = %q, want %q", got, "application/json")
	}
	if string(body) != `{"status":"SERVING"}` {
		t.Errorf("response body = %q, want %q", string(body), `{"status":"SERVING"}`)
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		t.Fatalf("Shutdown failed: %v", err)
	}

	serveErr := <-errCh
	if !errors.Is(serveErr, http.ErrServerClosed) {
		t.Errorf("Serve returned %v, want %v", serveErr, http.ErrServerClosed)
	}

	if _, err := http.Get("http://" + addr + "/healthz"); err == nil {
		t.Errorf("expected GET /healthz to fail after Shutdown, got nil error")
	}
}
