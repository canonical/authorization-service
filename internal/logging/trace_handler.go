// Copyright 2026 Canonical Ltd.
// SPDX-License-Identifier: AGPL-3.0-only

package logging

import (
	"context"
	"log/slog"

	"go.opentelemetry.io/otel/trace"
)

// TraceHandler wraps an slog.Handler, injecting trace_id/span_id attributes
// from the active OpenTelemetry span (if any) and request_id from context
// into every log record it handles, so logs can be correlated with traces and
// requests in Grafana/Tempo.
type TraceHandler struct {
	handler slog.Handler
}

// NewTraceHandler wraps handler with trace/span and request_id correlation.
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
	if reqID := RequestIDFromContext(ctx); reqID != "" {
		hasReqID := false
		r.Attrs(func(a slog.Attr) bool {
			if a.Key == "request_id" {
				hasReqID = true
				return false
			}
			return true
		})
		if !hasReqID {
			r.AddAttrs(slog.String("request_id", reqID))
		}
	}
	return h.handler.Handle(ctx, r)
}

func (h *TraceHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return NewTraceHandler(h.handler.WithAttrs(attrs))
}

func (h *TraceHandler) WithGroup(name string) slog.Handler {
	return NewTraceHandler(h.handler.WithGroup(name))
}
