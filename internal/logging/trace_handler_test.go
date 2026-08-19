// Copyright 2026 Canonical Ltd.
// SPDX-License-Identifier: AGPL-3.0-only

package logging

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
)

func newTestLogger() (*slog.Logger, *bytes.Buffer) {
	var buf bytes.Buffer
	handler := NewTraceHandler(slog.NewJSONHandler(&buf, nil))
	return slog.New(handler), &buf
}

func decodeLogLine(t *testing.T, buf *bytes.Buffer) map[string]any {
	t.Helper()
	var m map[string]any
	require.NoError(t, json.Unmarshal(buf.Bytes(), &m))
	return m
}

func TestTraceHandler_Handle_NoActiveSpan(t *testing.T) {
	logger, buf := newTestLogger()
	logger.InfoContext(context.Background(), "hello")

	entry := decodeLogLine(t, buf)
	assert.NotContains(t, entry, "trace_id")
	assert.NotContains(t, entry, "span_id")
}

func TestTraceHandler_Handle_WithActiveSpan(t *testing.T) {
	tp := sdktrace.NewTracerProvider()
	defer func() { _ = tp.Shutdown(context.Background()) }()
	tracer := tp.Tracer("test")

	ctx, span := tracer.Start(context.Background(), "test-span")
	defer span.End()

	logger, buf := newTestLogger()
	logger.InfoContext(ctx, "hello")

	entry := decodeLogLine(t, buf)
	assert.Equal(t, span.SpanContext().TraceID().String(), entry["trace_id"])
	assert.Equal(t, span.SpanContext().SpanID().String(), entry["span_id"])
}

func TestTraceHandler_WithAttrs_PreservesTraceBridge(t *testing.T) {
	tp := sdktrace.NewTracerProvider()
	defer func() { _ = tp.Shutdown(context.Background()) }()
	tracer := tp.Tracer("test")

	ctx, span := tracer.Start(context.Background(), "test-span")
	defer span.End()

	logger, buf := newTestLogger()
	scoped := logger.With("service", "authz")
	scoped.InfoContext(ctx, "hello")

	entry := decodeLogLine(t, buf)
	assert.Equal(t, "authz", entry["service"])
	assert.Equal(t, span.SpanContext().TraceID().String(), entry["trace_id"])
	assert.Equal(t, span.SpanContext().SpanID().String(), entry["span_id"])
}

func TestTraceHandler_WithGroup_PreservesTraceBridge(t *testing.T) {
	tp := sdktrace.NewTracerProvider()
	defer func() { _ = tp.Shutdown(context.Background()) }()
	tracer := tp.Tracer("test")

	ctx, span := tracer.Start(context.Background(), "test-span")
	defer span.End()

	logger, buf := newTestLogger()
	grouped := logger.WithGroup("http").With("method", "GET")
	grouped.InfoContext(ctx, "hello")

	entry := decodeLogLine(t, buf)
	require.Contains(t, entry, "http")
	group, ok := entry["http"].(map[string]any)
	require.True(t, ok)
	// trace_id/span_id are added via r.AddAttrs, so like any attribute logged
	// after WithGroup they nest under the open group rather than top-level.
	assert.Equal(t, "GET", group["method"])
	assert.Equal(t, span.SpanContext().TraceID().String(), group["trace_id"])
	assert.Equal(t, span.SpanContext().SpanID().String(), group["span_id"])
}
