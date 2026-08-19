// Copyright 2026 Canonical Ltd.
// SPDX-License-Identifier: AGPL-3.0-only

package grpc

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"

	"github.com/canonical/authorization-service/internal/logging"
	"github.com/canonical/authorization-service/internal/testutil"
)

// fakeServerTransportStream is a minimal grpc.ServerTransportStream fake so
// grpc.SetHeader has somewhere to record headers in tests.
type fakeServerTransportStream struct {
	header metadata.MD
}

func (f *fakeServerTransportStream) Method() string { return "/svc.Method/Call" }

func (f *fakeServerTransportStream) SetHeader(md metadata.MD) error {
	f.header = metadata.Join(f.header, md)
	return nil
}

func (f *fakeServerTransportStream) SendHeader(md metadata.MD) error { return f.SetHeader(md) }

func (f *fakeServerTransportStream) SetTrailer(metadata.MD) error { return nil }

func contextWithFakeStream(ctx context.Context) (context.Context, *fakeServerTransportStream) {
	stream := &fakeServerTransportStream{}
	return grpc.NewContextWithServerTransportStream(ctx, stream), stream
}

func decodeLogLine(t *testing.T, raw []byte) map[string]any {
	t.Helper()
	var entry map[string]any
	require.NoError(t, json.Unmarshal(raw, &entry))
	return entry
}

func TestRequestIDInterceptor_GeneratesIDWhenAbsent(t *testing.T) {
	ctx, stream := contextWithFakeStream(context.Background())
	info := &grpc.UnaryServerInfo{FullMethod: "/svc.Method/Call"}

	var seenRequestID string
	handler := func(ctx context.Context, _ interface{}) (interface{}, error) {
		seenRequestID = logging.RequestIDFromContext(ctx)
		return nil, nil
	}

	_, err := requestIDInterceptor()(ctx, nil, info, handler)
	require.NoError(t, err)

	require.NotEmpty(t, seenRequestID)
	assert.Equal(t, seenRequestID, stream.header.Get(requestIDMetadataKey)[0])
}

func TestRequestIDInterceptor_ReusesIncomingID(t *testing.T) {
	ctx, stream := contextWithFakeStream(context.Background())
	ctx = metadata.NewIncomingContext(ctx, metadata.Pairs(requestIDMetadataKey, "req-123"))
	info := &grpc.UnaryServerInfo{FullMethod: "/svc.Method/Call"}

	var seenRequestID string
	handler := func(ctx context.Context, _ interface{}) (interface{}, error) {
		seenRequestID = logging.RequestIDFromContext(ctx)
		return nil, nil
	}

	_, err := requestIDInterceptor()(ctx, nil, info, handler)
	require.NoError(t, err)

	assert.Equal(t, "req-123", seenRequestID)
	assert.Equal(t, "req-123", stream.header.Get(requestIDMetadataKey)[0])
}

func TestLoggingInterceptor_LogsRequestFields(t *testing.T) {
	logger, buf := testutil.CapturingLogger(t)
	ctx := logging.ContextWithRequestID(context.Background(), "req-abc")
	info := &grpc.UnaryServerInfo{FullMethod: "/svc.Method/Call"}
	handler := func(ctx context.Context, _ interface{}) (interface{}, error) {
		return nil, status.Error(codes.NotFound, "nope")
	}

	_, err := loggingInterceptor(logger)(ctx, nil, info, handler)
	require.Error(t, err)

	entry := decodeLogLine(t, buf.Bytes())
	assert.Equal(t, "gRPC request", entry["msg"])
	assert.Equal(t, info.FullMethod, entry["method"])
	assert.Equal(t, "req-abc", entry["request_id"])
	assert.Equal(t, float64(codes.NotFound), entry["code"])
	assert.Contains(t, entry, "duration_ms")
}

func TestRecoveryInterceptor_RecoversPanicAndLogs(t *testing.T) {
	logger, buf := testutil.CapturingLogger(t)
	ctx := logging.ContextWithRequestID(context.Background(), "req-abc")
	info := &grpc.UnaryServerInfo{FullMethod: "/svc.Method/Call"}
	handler := func(context.Context, interface{}) (interface{}, error) {
		panic("boom")
	}

	_, err := recoveryInterceptor(logger)(ctx, nil, info, handler)
	require.Error(t, err)
	st, ok := status.FromError(err)
	require.True(t, ok)
	assert.Equal(t, codes.Internal, st.Code())

	entry := decodeLogLine(t, buf.Bytes())
	assert.Equal(t, "Panic recovered", entry["msg"])
	assert.Equal(t, "req-abc", entry["request_id"])
	assert.Equal(t, "boom", entry["panic"])
}

func TestRecoveryInterceptor_NoPanicPassesThrough(t *testing.T) {
	logger, buf := testutil.CapturingLogger(t)
	info := &grpc.UnaryServerInfo{FullMethod: "/svc.Method/Call"}
	wantErr := errors.New("boom")
	handler := func(context.Context, interface{}) (interface{}, error) {
		return "resp", wantErr
	}

	resp, err := recoveryInterceptor(logger)(context.Background(), nil, info, handler)
	assert.Equal(t, "resp", resp)
	assert.Equal(t, wantErr, err)
	assert.Empty(t, buf.Bytes())
}
