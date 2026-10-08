// Copyright 2026 Canonical Ltd.
// SPDX-License-Identifier: AGPL-3.0-only

package sts

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
	"google.golang.org/grpc"

	stsv1 "github.com/canonical/authorization-service/client/v1/sts"
)

var _ stsv1.SecurityTokenServiceClient = (*STSClientWrapper)(nil)

// STSClientWrapper wraps the SecurityTokenServiceClient with tracing and logging
type STSClientWrapper struct {
	delegate stsv1.SecurityTokenServiceClient
	logger   *slog.Logger
	tracer   trace.Tracer
}

func NewSTSClientWrapper(client stsv1.SecurityTokenServiceClient, logger *slog.Logger, tracer trace.Tracer) *STSClientWrapper {
	return &STSClientWrapper{
		delegate: client,
		logger:   logger,
		tracer:   tracer,
	}
}

func (w *STSClientWrapper) ExchangeSession(ctx context.Context, in *stsv1.ExchangeRequest, opts ...grpc.CallOption) (*stsv1.ExchangeResponse, error) {
	ctx, span := w.tracer.Start(ctx, "service.sts.ExchangeSession")
	defer span.End()

	start := time.Now()
	w.logger.Debug("Calling STS ExchangeSession",
		"session_cookie", in.GetSessionCookie(),
	)

	resp, err := w.delegate.ExchangeSession(ctx, in, opts...)
	duration := time.Since(start)

	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		w.logger.Error("STS ExchangeSession failed",
			"error", err,
			"duration_ms", duration.Milliseconds(),
		)
		return nil, err
	}

	span.SetAttributes(
		attribute.String("access_token_present", fmt.Sprintf("%t", resp.GetAccessToken() != "")),
		attribute.Int64("expires_in", resp.GetExpiresIn()),
	)
	span.SetStatus(codes.Ok, "Success")

	w.logger.Debug("STS ExchangeSession succeeded",
		"expires_in", resp.GetExpiresIn(),
		"duration_ms", duration.Milliseconds(),
	)

	return resp, nil
}

func (w *STSClientWrapper) ExchangeToken(ctx context.Context, in *stsv1.ExchangeTokenRequest, opts ...grpc.CallOption) (*stsv1.ExchangeResponse, error) {
	ctx, span := w.tracer.Start(ctx, "service.sts.ExchangeToken")
	defer span.End()

	start := time.Now()
	w.logger.Debug("Calling STS ExchangeToken")

	resp, err := w.delegate.ExchangeToken(ctx, in, opts...)
	duration := time.Since(start)

	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		w.logger.Error("STS ExchangeToken failed",
			"error", err,
			"duration_ms", duration.Milliseconds(),
		)
		return nil, err
	}

	span.SetAttributes(
		attribute.String("access_token_present", fmt.Sprintf("%t", resp.GetAccessToken() != "")),
		attribute.Int64("expires_in", resp.GetExpiresIn()),
	)
	span.SetStatus(codes.Ok, "Success")

	w.logger.Debug("STS ExchangeToken succeeded",
		"expires_in", resp.GetExpiresIn(),
		"duration_ms", duration.Milliseconds(),
	)

	return resp, nil
}

func (w *STSClientWrapper) RevokeUserSessions(ctx context.Context, in *stsv1.RevokeUserRequest, opts ...grpc.CallOption) (*stsv1.RevokeUserResponse, error) {
	ctx, span := w.tracer.Start(ctx, "service.sts.RevokeUserSessions",
		trace.WithAttributes(
			attribute.String("user_id", in.GetUserId()),
		),
	)
	defer span.End()

	start := time.Now()
	w.logger.Debug("Calling STS RevokeUserSessions",
		"user_id", in.GetUserId(),
	)

	resp, err := w.delegate.RevokeUserSessions(ctx, in, opts...)
	duration := time.Since(start)

	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		w.logger.Error("STS RevokeUserSessions failed",
			"error", err,
			"user_id", in.GetUserId(),
			"duration_ms", duration.Milliseconds(),
		)
		return nil, err
	}

	span.SetAttributes(
		attribute.Bool("success", resp.GetSuccess()),
	)
	span.SetStatus(codes.Ok, "Success")

	w.logger.Info("STS RevokeUserSessions succeeded",
		"user_id", in.GetUserId(),
		"success", resp.GetSuccess(),
		"duration_ms", duration.Milliseconds(),
	)

	return resp, nil
}
