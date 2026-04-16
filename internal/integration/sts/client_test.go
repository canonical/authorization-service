package sts

import (
	"context"
	"errors"
	"log/slog"
	"testing"

	"go.opentelemetry.io/otel/trace/noop"
	"google.golang.org/grpc"

	stsv1 "github.com/canonical/authorization-service/client/v1/sts"
)

// mockSTSClient is a hand-written mock for stsv1.SecurityTokenServiceClient.
type mockSTSClient struct {
	exchangeResp *stsv1.ExchangeResponse
	exchangeErr  error
	revokeResp   *stsv1.RevokeUserResponse
	revokeErr    error
}

func (m *mockSTSClient) ExchangeSession(_ context.Context, _ *stsv1.ExchangeRequest, _ ...grpc.CallOption) (*stsv1.ExchangeResponse, error) {
	return m.exchangeResp, m.exchangeErr
}

func (m *mockSTSClient) RevokeUserSessions(_ context.Context, _ *stsv1.RevokeUserRequest, _ ...grpc.CallOption) (*stsv1.RevokeUserResponse, error) {
	return m.revokeResp, m.revokeErr
}

func newTestWrapper(mock *mockSTSClient) *STSClientWrapper {
	logger := slog.Default()
	tracer := noop.NewTracerProvider().Tracer("test")
	return NewSTSClientWrapper(mock, logger, tracer)
}

func TestNewSTSClientWrapper(t *testing.T) {
	mock := &mockSTSClient{}
	w := newTestWrapper(mock)
	if w == nil {
		t.Fatal("expected non-nil wrapper")
	}
}

func TestExchangeSession_Success(t *testing.T) {
	mock := &mockSTSClient{
		exchangeResp: &stsv1.ExchangeResponse{
			AccessToken: "tok",
			ExpiresIn:   3600,
		},
	}
	w := newTestWrapper(mock)

	resp, err := w.ExchangeSession(context.Background(), &stsv1.ExchangeRequest{SessionCookie: "cookie"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.GetAccessToken() != "tok" {
		t.Errorf("expected token 'tok', got %q", resp.GetAccessToken())
	}
	if resp.GetExpiresIn() != 3600 {
		t.Errorf("expected expires_in 3600, got %d", resp.GetExpiresIn())
	}
}

func TestExchangeSession_EmptyToken(t *testing.T) {
	mock := &mockSTSClient{
		exchangeResp: &stsv1.ExchangeResponse{
			AccessToken: "",
			ExpiresIn:   0,
		},
	}
	w := newTestWrapper(mock)

	resp, err := w.ExchangeSession(context.Background(), &stsv1.ExchangeRequest{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.GetAccessToken() != "" {
		t.Errorf("expected empty token")
	}
}

func TestExchangeSession_Error(t *testing.T) {
	mock := &mockSTSClient{
		exchangeErr: errors.New("exchange failed"),
	}
	w := newTestWrapper(mock)

	_, err := w.ExchangeSession(context.Background(), &stsv1.ExchangeRequest{SessionCookie: "cookie"})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if err.Error() != "exchange failed" {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestRevokeUserSessions_Success(t *testing.T) {
	mock := &mockSTSClient{
		revokeResp: &stsv1.RevokeUserResponse{Success: true},
	}
	w := newTestWrapper(mock)

	resp, err := w.RevokeUserSessions(context.Background(), &stsv1.RevokeUserRequest{UserId: "user-1"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !resp.GetSuccess() {
		t.Error("expected success=true")
	}
}

func TestRevokeUserSessions_NotSuccessful(t *testing.T) {
	mock := &mockSTSClient{
		revokeResp: &stsv1.RevokeUserResponse{Success: false},
	}
	w := newTestWrapper(mock)

	resp, err := w.RevokeUserSessions(context.Background(), &stsv1.RevokeUserRequest{UserId: "user-1"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.GetSuccess() {
		t.Error("expected success=false")
	}
}

func TestRevokeUserSessions_Error(t *testing.T) {
	mock := &mockSTSClient{
		revokeErr: errors.New("revoke failed"),
	}
	w := newTestWrapper(mock)

	_, err := w.RevokeUserSessions(context.Background(), &stsv1.RevokeUserRequest{UserId: "user-1"})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if err.Error() != "revoke failed" {
		t.Errorf("unexpected error: %v", err)
	}
}
