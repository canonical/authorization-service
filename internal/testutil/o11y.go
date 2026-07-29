// Copyright 2026 Canonical Ltd.
// SPDX-License-Identifier: AGPL-3.0-only

package testutil

import (
	"io"
	"log/slog"
	"testing"

	"go.opentelemetry.io/otel/trace"
	"go.opentelemetry.io/otel/trace/noop"
)

func TestLogger(t *testing.T) *slog.Logger {
	t.Helper()
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func TestTracer(t *testing.T) trace.Tracer {
	t.Helper()
	return noop.NewTracerProvider().Tracer("test")
}
