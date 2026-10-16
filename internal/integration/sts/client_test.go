// Copyright 2026 Canonical Ltd.
// SPDX-License-Identifier: AGPL-3.0-only

package sts

import (
	"context"
	"errors"
	"log/slog"
	"net"
	"testing"
	"time"

	"go.opentelemetry.io/otel/trace/noop"
	"google.golang.org/grpc"

	stsv1 "github.com/canonical/authorization-service/client/v1/sts"
)

func TestCreateConnection_InsecureAndTLS(t *testing.T) {
	conn, err := CreateConnection("localhost:9090", false, time.Second, false)
	if err != nil {
		t.Fatalf("unexpected error creating insecure connection: %v", err)
	}
	_ = conn.Close()

	tlsConn, err := CreateConnection("localhost:9090", true, time.Second, false)
	if err != nil {
		t.Fatalf("unexpected error creating TLS connection: %v", err)
	}
	_ = tlsConn.Close()
}

func TestCreateConnection_DialError(t *testing.T) {
	_, err := CreateConnection("localhost:9090", false, time.Second, false, grpc.WithDefaultServiceConfig("{invalid-json"))
	if err == nil {
		t.Fatal("expected error for invalid service config dial option, got nil")
	}
}

func TestCreateConnection_EagerCheckSuccess(t *testing.T) {
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to listen: %v", err)
	}
	srv := grpc.NewServer()
	go func() {
		_ = srv.Serve(lis)
	}()
	defer srv.Stop()

	conn, err := CreateConnection(lis.Addr().String(), false, 2*time.Second, true)
	if err != nil {
		t.Fatalf("expected eager connection to succeed, got: %v", err)
	}
	_ = conn.Close()
}

func TestCreateConnection_EagerCheckTimeout(t *testing.T) {
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to listen: %v", err)
	}
	addr := lis.Addr().String()
	_ = lis.Close() // closed port so connection never reaches Ready

	_, err = CreateConnection(addr, false, 50*time.Millisecond, true)
	if err == nil {
		t.Fatal("expected timeout error on closed port with eagerCheck=true, got nil")
	}
}

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

func (m *mockSTSClient) ExchangeToken(_ context.Context, _ *stsv1.ExchangeTokenRequest, _ ...grpc.CallOption) (*stsv1.ExchangeResponse, error) {
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

func TestExchangeToken_Success(t *testing.T) {
	mock := &mockSTSClient{
		exchangeResp: &stsv1.ExchangeResponse{
			AccessToken: "m2m-token",
			ExpiresIn:   3600,
		},
	}
	w := newTestWrapper(mock)

	resp, err := w.ExchangeToken(context.Background(), &stsv1.ExchangeTokenRequest{Token: "hydra-access-token"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.GetAccessToken() != "m2m-token" {
		t.Errorf("expected token 'm2m-token', got %q", resp.GetAccessToken())
	}
	if resp.GetExpiresIn() != 3600 {
		t.Errorf("expected expires_in 3600, got %d", resp.GetExpiresIn())
	}
}

func TestExchangeToken_Error(t *testing.T) {
	mock := &mockSTSClient{
		exchangeErr: errors.New("exchange token failed"),
	}
	w := newTestWrapper(mock)

	_, err := w.ExchangeToken(context.Background(), &stsv1.ExchangeTokenRequest{Token: "bad-token"})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if err.Error() != "exchange token failed" {
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
