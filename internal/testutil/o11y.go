// Copyright 2026 Canonical Ltd.
// SPDX-License-Identifier: AGPL-3.0-only

package testutil

import (
	"bytes"
	"io"
	"log/slog"
	"testing"

	"go.opentelemetry.io/otel/trace"
	"go.opentelemetry.io/otel/trace/noop"

	"github.com/canonical/authorization-service/internal/logging"
)

func TestLogger(t *testing.T) *slog.Logger {
	t.Helper()
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func TestTracer(t *testing.T) trace.Tracer {
	t.Helper()
	return noop.NewTracerProvider().Tracer("test")
}

// CapturingLogger returns a logger using the same JSON+TraceHandler stack as
// production, writing to an in-memory buffer so tests can assert on the
// fields actually logged.
func CapturingLogger(t *testing.T) (*slog.Logger, *bytes.Buffer) {
	t.Helper()
	var buf bytes.Buffer
	handler := logging.NewTraceHandler(slog.NewJSONHandler(&buf, nil))
	return slog.New(handler), &buf
}
