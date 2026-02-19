//go:generate mockgen -source=../../integrations/openfga/client.go -destination=mocks/mock_openfga.go -package=authz
//go:generate mockgen -source=../../integrations/valkey/client.go -destination=mocks/mock_valkey.go -package=authz
//go:generate mockgen -source=../../integrations/sts/client.go -destination=mocks/mock_sts.go -package=authz

package authz

import (
    "context"
    "io"
    "log/slog"
    "testing"

    gomock "go.uber.org/mock/gomock"

    "github.com/canonical/authorization-service/internal/integrations/openfga"
    authz "github.com/canonical/authorization-service/internal/service/authz/mocks"
)

func testLogger(t *testing.T) *slog.Logger {
    return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func TestServiceCheckAuthorizationAllowed(t *testing.T) {
    ctrl := gomock.NewController(t)
    defer ctrl.Finish()

    mockFGA := authz.NewMockClientInterface(ctrl)
    mockFGA.EXPECT().
        Check(gomock.Any(), gomock.Any()).
        Return(&openfga.CheckResponse{Allowed: true}, nil).
        Times(1)

    svc := NewService(mockFGA, testLogger(t))

    var (
        reqUser     = "user:alice"
        reqResource = "resource:doc1"
        reqAction   = "read"
    )

    resp, err := svc.Check(context.Background(), reqUser, reqResource, reqAction)

    if err != nil {
        t.Fatalf("Check failed: %v", err)
    }
    if !resp.Allowed {
        t.Error("expected authorization to be allowed")
    }
    if resp.Reason != "evaluated" {
        t.Errorf("expected reason %q, got %q", "evaluated", resp.Reason)
    }
}

func TestServiceCheckAuthorizationDenied(t *testing.T) {
    ctrl := gomock.NewController(t)
    defer ctrl.Finish()

    mockFGA := authz.NewMockClientInterface(ctrl)
    mockFGA.EXPECT().
        Check(gomock.Any(), gomock.Any()).
        Return(&openfga.CheckResponse{Allowed: false}, nil).
        Times(1)

    svc := NewService(mockFGA, testLogger(t))

    var (
        reqUser     = "user:bob"
        reqResource = "resource:secret"
        reqAction   = "admin"
    )

    resp, err := svc.Check(context.Background(), reqUser, reqResource, reqAction)

    if err != nil {
        t.Fatalf("Check failed: %v", err)
    }
    if resp.Allowed {
        t.Error("expected authorization to be denied")
    }
}

func TestServiceGrantAccess(t *testing.T) {
    ctrl := gomock.NewController(t)
    defer ctrl.Finish()

    mockFGA := authz.NewMockClientInterface(ctrl)
    mockFGA.EXPECT().
        Write(gomock.Any(), gomock.Any()).
        Do(func(ctx context.Context, req *openfga.WriteRequest) {
            if len(req.Writes) != 1 {
                t.Errorf("expected 1 write, got %d", len(req.Writes))
            }
            if req.Writes[0].User != "user:alice" {
                t.Errorf("expected user %q, got %q", "user:alice", req.Writes[0].User)
            }
            if req.Writes[0].Object != "resource:doc1" {
                t.Errorf("expected object %q, got %q", "resource:doc1", req.Writes[0].Object)
            }
            if req.Writes[0].Relation != "read" {
                t.Errorf("expected relation %q, got %q", "read", req.Writes[0].Relation)
            }
        }).
        Return(&openfga.WriteResponse{Success: true}, nil).
        Times(1)

    svc := NewService(mockFGA, testLogger(t))

    err := svc.GrantAccess(context.Background(), "user:alice", "resource:doc1", "read")

    if err != nil {
        t.Fatalf("GrantAccess failed: %v", err)
    }
}

func TestServiceRevokeAccess(t *testing.T) {
    ctrl := gomock.NewController(t)
    defer ctrl.Finish()

    mockFGA := authz.NewMockClientInterface(ctrl)
    mockFGA.EXPECT().
        Write(gomock.Any(), gomock.Any()).
        Do(func(ctx context.Context, req *openfga.WriteRequest) {
            if len(req.Deletes) != 1 {
                t.Errorf("expected 1 delete, got %d", len(req.Deletes))
            }
            if req.Deletes[0].User != "user:alice" {
                t.Errorf("expected user %q, got %q", "user:alice", req.Deletes[0].User)
            }
            if req.Deletes[0].Object != "resource:doc1" {
                t.Errorf("expected object %q, got %q", "resource:doc1", req.Deletes[0].Object)
            }
        }).
        Return(&openfga.WriteResponse{Success: true}, nil).
        Times(1)

    svc := NewService(mockFGA, testLogger(t))

    err := svc.RevokeAccess(context.Background(), "user:alice", "resource:doc1", "read")

    if err != nil {
        t.Fatalf("RevokeAccess failed: %v", err)
    }
}
