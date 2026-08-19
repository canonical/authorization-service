// Copyright 2026 Canonical Ltd.
// SPDX-License-Identifier: AGPL-3.0-only

package grpc

import (
	"context"
	"log/slog"
	"runtime/debug"
	"time"

	"github.com/google/uuid"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"

	"github.com/canonical/authorization-service/internal/logging"
)

// requestIDMetadataKey is the gRPC metadata key carrying the request ID,
// read from an incoming call (typically set by the mesh/Envoy at the edge)
// or generated when absent, and echoed back as a response header.
const requestIDMetadataKey = "x-request-id"

// requestIDInterceptor establishes the request ID for the call: it reads
// requestIDMetadataKey from incoming metadata, generating one if absent,
// stores it in the context for downstream logging, and echoes it back as a
// response header so callers can correlate their own logs. It must run
// first in the chain so every later interceptor can log the request ID.
func requestIDInterceptor() grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
		requestID := requestIDFromIncomingContext(ctx)
		if requestID == "" {
			requestID = uuid.NewString()
		}

		ctx = logging.ContextWithRequestID(ctx, requestID)
		_ = grpc.SetHeader(ctx, metadata.Pairs(requestIDMetadataKey, requestID))

		return handler(ctx, req)
	}
}

func requestIDFromIncomingContext(ctx context.Context) string {
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return ""
	}
	values := md.Get(requestIDMetadataKey)
	if len(values) == 0 {
		return ""
	}
	return values[0]
}

// loggingInterceptor logs all gRPC requests
func loggingInterceptor(logger *slog.Logger) grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
		start := time.Now()

		resp, err := handler(ctx, req)

		duration := time.Since(start)
		code := codes.OK
		if err != nil {
			if st, ok := status.FromError(err); ok {
				code = st.Code()
			} else {
				code = codes.Unknown
			}
		}

		logger.InfoContext(ctx, "gRPC request",
			"method", info.FullMethod,
			"request_id", logging.RequestIDFromContext(ctx),
			"duration_ms", duration.Milliseconds(),
			"code", code,
		)

		return resp, err
	}
}

// recoveryInterceptor recovers from panics
func recoveryInterceptor(logger *slog.Logger) grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (resp interface{}, err error) {
		defer func() {
			if r := recover(); r != nil {
				logger.ErrorContext(ctx, "Panic recovered",
					"method", info.FullMethod,
					"request_id", logging.RequestIDFromContext(ctx),
					"panic", r,
					"stack", string(debug.Stack()),
				)
				err = status.Errorf(codes.Internal, "internal server error")
			}
		}()

		return handler(ctx, req)
	}
}
