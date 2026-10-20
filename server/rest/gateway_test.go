// Copyright 2026 Canonical Ltd.
// SPDX-License-Identifier: AGPL-3.0-only

package rest

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/canonical/authorization-service/internal/logging"
	"github.com/canonical/authorization-service/internal/metrics"
	"github.com/canonical/authorization-service/internal/testutil"
)

type stubServerConfig struct {
	addr string
}

func (s stubServerConfig) GetHTTPAddress() string {
	return s.addr
}

func TestNewGateway_HealthzEndpoint(t *testing.T) {
	logger, _ := testutil.CapturingLogger(t)
	gw, err := NewGateway(stubServerConfig{addr: "127.0.0.1:0"}, metrics.NewRegistry(), logger)
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	rec := httptest.NewRecorder()

	gw.server.Handler.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, "application/json", rec.Header().Get("Content-Type"))
	assert.Equal(t, `{"status":"SERVING"}`, rec.Body.String())
}

func decodeLogLine(t *testing.T, raw []byte) map[string]any {
	t.Helper()
	var entry map[string]any
	require.NoError(t, json.Unmarshal(raw, &entry))
	return entry
}

func TestRequestIDMiddleware_GeneratesIDWhenAbsent(t *testing.T) {
	var seenRequestID string
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seenRequestID = logging.RequestIDFromContext(r.Context())
	})

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()

	requestIDMiddleware(next).ServeHTTP(rec, req)

	require.NotEmpty(t, seenRequestID)
	assert.Equal(t, seenRequestID, rec.Header().Get(requestIDHeader))
	assert.Equal(t, seenRequestID, req.Header.Get(requestIDHeader))
}

func TestRequestIDMiddleware_ReusesIncomingID(t *testing.T) {
	var seenRequestID string
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seenRequestID = logging.RequestIDFromContext(r.Context())
	})

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set(requestIDHeader, "req-123")
	rec := httptest.NewRecorder()

	requestIDMiddleware(next).ServeHTTP(rec, req)

	assert.Equal(t, "req-123", seenRequestID)
	assert.Equal(t, "req-123", rec.Header().Get(requestIDHeader))
}

func TestRequestIDAnnotator_ForwardsIncomingHeader(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set(requestIDHeader, "req-123")

	md := requestIDAnnotator(req.Context(), req)

	require.NotNil(t, md)
	assert.Equal(t, []string{"req-123"}, md.Get(requestIDMetadataKey))
}

func TestRequestIDAnnotator_NoHeaderReturnsNil(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)

	md := requestIDAnnotator(req.Context(), req)

	assert.Nil(t, md)
}

func TestLoggingMiddleware_LogsRequestFields(t *testing.T) {
	logger, buf := testutil.CapturingLogger(t)
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	})

	req := httptest.NewRequest(http.MethodGet, "/foo/bar", nil)
	req = req.WithContext(logging.ContextWithRequestID(req.Context(), "req-abc"))
	rec := httptest.NewRecorder()

	loggingMiddleware(next, logger).ServeHTTP(rec, req)

	entry := decodeLogLine(t, buf.Bytes())
	assert.Equal(t, "HTTP request", entry["msg"])
	assert.Equal(t, http.MethodGet, entry["method"])
	assert.Equal(t, "/foo/bar", entry["path"])
	assert.Equal(t, float64(http.StatusNotFound), entry["status"])
	assert.Equal(t, "req-abc", entry["request_id"])
	assert.Contains(t, entry, "duration_ms")
}

func TestLoggingMiddleware_DefaultsStatusOKWhenNotWritten(t *testing.T) {
	logger, buf := testutil.CapturingLogger(t)
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {})

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()

	loggingMiddleware(next, logger).ServeHTTP(rec, req)

	entry := decodeLogLine(t, buf.Bytes())
	assert.Equal(t, float64(http.StatusOK), entry["status"])
}
