// Copyright 2026 Canonical Ltd.
// SPDX-License-Identifier: AGPL-3.0-only

//go:generate mockgen -source=../../integration/valkey/client.go -destination=mocks/mock_valkey.go -package=permissions

package permissions

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"
	"time"

	gomock "go.uber.org/mock/gomock"

	permissions "github.com/canonical/authorization-service/internal/service/permissions/mocks"
)

func testLogger(t *testing.T) *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

// operationMetricsCall captures a single ObserveOperation invocation.
type operationMetricsCall struct {
	operation string
	err       error
}

// fakeOperationMetrics is a hand-rolled Metrics fake recording every
// ObserveOperation call so tests can assert operation label and outcome.
type fakeOperationMetrics struct {
	calls []operationMetricsCall
}

func (f *fakeOperationMetrics) ObserveOperation(operation string, err error, _ time.Duration) {
	f.calls = append(f.calls, operationMetricsCall{operation: operation, err: err})
}

// assertObserveOperation asserts ObserveOperation was called exactly once
// with the given operation and error-presence.
func assertObserveOperation(t *testing.T, m *fakeOperationMetrics, wantOperation string, wantErr bool) {
	t.Helper()
	if len(m.calls) != 1 {
		t.Fatalf("expected exactly 1 ObserveOperation call, got %d: %+v", len(m.calls), m.calls)
	}
	got := m.calls[0]
	if got.operation != wantOperation {
		t.Errorf("ObserveOperation operation = %q, want %q", got.operation, wantOperation)
	}
	if (got.err != nil) != wantErr {
		t.Errorf("ObserveOperation err = %v, want non-nil = %t", got.err, wantErr)
	}
}

func TestServiceRegister(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockCache := permissions.NewMockCacheClientInterface(ctrl)
	mockCache.EXPECT().
		Set(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		Return(nil).
		Times(1)

	metrics := &fakeOperationMetrics{}
	svc := NewService(mockCache, metrics, testLogger(t))

	perm, err := svc.Register(context.Background(), "test-service", "Test Service",
		map[string]interface{}{"read": true, "write": false}, "v1")

	if err != nil {
		t.Fatalf("Register failed: %v", err)
	}
	if perm == nil {
		t.Fatal("expected permission to be returned")
	}
	if perm.ServiceID != "test-service" {
		t.Errorf("expected service_id %q, got %q", "test-service", perm.ServiceID)
	}
	if perm.ServiceName != "Test Service" {
		t.Errorf("expected service_name %q, got %q", "Test Service", perm.ServiceName)
	}
	if perm.Version != "v1" {
		t.Errorf("expected version %q, got %q", "v1", perm.Version)
	}
	assertObserveOperation(t, metrics, "register", false)
}

func TestServiceRegister_CacheSetError(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockCache := permissions.NewMockCacheClientInterface(ctrl)
	mockCache.EXPECT().
		Set(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		Return(errors.New("cache unavailable")).
		Times(1)

	metrics := &fakeOperationMetrics{}
	svc := NewService(mockCache, metrics, testLogger(t))

	perm, err := svc.Register(context.Background(), "test-service", "Test Service",
		map[string]interface{}{"read": true}, "v1")

	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if perm != nil {
		t.Errorf("expected nil permission, got %v", perm)
	}
	assertObserveOperation(t, metrics, "register", true)
}

func TestServiceGet(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockCache := permissions.NewMockCacheClientInterface(ctrl)
	mockCache.EXPECT().
		Get(gomock.Any(), "permissions:test-service").
		Return(`{"id":"test-id","service_id":"test-service","service_name":"Test Service","permissions":{"read":true},"version":"v1","registered_at":"2024-01-01T00:00:00Z"}`, nil).
		Times(1)

	metrics := &fakeOperationMetrics{}
	svc := NewService(mockCache, metrics, testLogger(t))

	perm, err := svc.Get(context.Background(), "test-service")

	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}
	if perm == nil {
		t.Fatal("expected permission to be returned")
	}
	if perm.ServiceID != "test-service" {
		t.Errorf("expected service_id %q, got %q", "test-service", perm.ServiceID)
	}
	assertObserveOperation(t, metrics, "get", false)
}

func TestServiceGet_CacheGetError(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockCache := permissions.NewMockCacheClientInterface(ctrl)
	mockCache.EXPECT().
		Get(gomock.Any(), "permissions:test-service").
		Return("", errors.New("not found")).
		Times(1)

	metrics := &fakeOperationMetrics{}
	svc := NewService(mockCache, metrics, testLogger(t))

	perm, err := svc.Get(context.Background(), "test-service")

	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if perm != nil {
		t.Errorf("expected nil permission, got %v", perm)
	}
	assertObserveOperation(t, metrics, "get", true)
}

func TestServiceGet_UnmarshalError(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockCache := permissions.NewMockCacheClientInterface(ctrl)
	mockCache.EXPECT().
		Get(gomock.Any(), "permissions:test-service").
		Return("not valid json", nil).
		Times(1)

	metrics := &fakeOperationMetrics{}
	svc := NewService(mockCache, metrics, testLogger(t))

	perm, err := svc.Get(context.Background(), "test-service")

	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if perm != nil {
		t.Errorf("expected nil permission, got %v", perm)
	}
	assertObserveOperation(t, metrics, "get", true)
}

func TestServiceDelete(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockCache := permissions.NewMockCacheClientInterface(ctrl)
	mockCache.EXPECT().
		Delete(gomock.Any(), "permissions:test-service").
		Return(nil).
		Times(1)

	metrics := &fakeOperationMetrics{}
	svc := NewService(mockCache, metrics, testLogger(t))

	err := svc.Delete(context.Background(), "test-service")

	if err != nil {
		t.Fatalf("Delete failed: %v", err)
	}
	assertObserveOperation(t, metrics, "delete", false)
}

func TestServiceDelete_CacheDeleteError(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockCache := permissions.NewMockCacheClientInterface(ctrl)
	mockCache.EXPECT().
		Delete(gomock.Any(), "permissions:test-service").
		Return(errors.New("cache unavailable")).
		Times(1)

	metrics := &fakeOperationMetrics{}
	svc := NewService(mockCache, metrics, testLogger(t))

	err := svc.Delete(context.Background(), "test-service")

	if err == nil {
		t.Fatal("expected error, got nil")
	}
	assertObserveOperation(t, metrics, "delete", true)
}
