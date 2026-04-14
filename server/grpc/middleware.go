package grpc

import (
	"context"
	"log/slog"
	"runtime/debug"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

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

		logger.Info("gRPC request",
			"method", info.FullMethod,
			"duration", duration,
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
				logger.Error("Panic recovered",
					"method", info.FullMethod,
					"panic", r,
					"stack", string(debug.Stack()),
				)
				err = status.Errorf(codes.Internal, "internal server error")
			}
		}()

		return handler(ctx, req)
	}
}
