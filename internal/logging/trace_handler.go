// Copyright 2026 Canonical Ltd.
// SPDX-License-Identifier: AGPL-3.0-only

package logging

import (
	"context"
	"log/slog"

	"go.opentelemetry.io/otel/trace"
)

// TraceHandler wraps an slog.Handler, injecting trace_id/span_id attributes
// from the active OpenTelemetry span (if any) into every log record it
// handles, so logs can be correlated with traces in Grafana/Tempo.
type TraceHandler struct {
	handler slog.Handler
}

// NewTraceHandler wraps handler with trace/span correlation.
func NewTraceHandler(handler slog.Handler) *TraceHandler {
	return &TraceHandler{handler: handler}
}

func (h *TraceHandler) Enabled(ctx context.Context, level slog.Level) bool {
	return h.handler.Enabled(ctx, level)
}

func (h *TraceHandler) Handle(ctx context.Context, r slog.Record) error {
	if sc := trace.SpanContextFromContext(ctx); sc.IsValid() {
		r.AddAttrs(
			slog.String("trace_id", sc.TraceID().String()),
			slog.String("span_id", sc.SpanID().String()),
		)
	}
	return h.handler.Handle(ctx, r)
}

func (h *TraceHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return NewTraceHandler(h.handler.WithAttrs(attrs))
}

func (h *TraceHandler) WithGroup(name string) slog.Handler {
	return NewTraceHandler(h.handler.WithGroup(name))
}
