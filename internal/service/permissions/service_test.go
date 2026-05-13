// Copyright 2026 Canonical Ltd.
// SPDX-License-Identifier: AGPL-3.0-only

//go:generate mockgen -source=../../integration/valkey/client.go -destination=mocks/mock_valkey.go -package=permissions

package permissions

import (
	"context"
	"io"
	"log/slog"
	"testing"

	gomock "go.uber.org/mock/gomock"

	permissions "github.com/canonical/authorization-service/internal/service/permissions/mocks"
)

func testLogger(t *testing.T) *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func TestServiceRegister(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockCache := permissions.NewMockCacheClientInterface(ctrl)
	mockCache.EXPECT().
		Set(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		Return(nil).
		Times(1)

	svc := NewService(mockCache, testLogger(t))

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
}

func TestServiceGet(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockCache := permissions.NewMockCacheClientInterface(ctrl)
	mockCache.EXPECT().
		Get(gomock.Any(), "permissions:test-service").
		Return(`{"id":"test-id","service_id":"test-service","service_name":"Test Service","permissions":{"read":true},"version":"v1","registered_at":"2024-01-01T00:00:00Z"}`, nil).
		Times(1)

	svc := NewService(mockCache, testLogger(t))

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
}

func TestServiceDelete(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockCache := permissions.NewMockCacheClientInterface(ctrl)
	mockCache.EXPECT().
		Delete(gomock.Any(), "permissions:test-service").
		Return(nil).
		Times(1)

	svc := NewService(mockCache, testLogger(t))

	err := svc.Delete(context.Background(), "test-service")

	if err != nil {
		t.Fatalf("Delete failed: %v", err)
	}
}
