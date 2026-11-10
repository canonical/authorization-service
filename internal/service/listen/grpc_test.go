// Copyright 2026 Canonical Ltd.
// SPDX-License-Identifier: AGPL-3.0-only

package listen

import (
	"context"
	"errors"
	"testing"

	"github.com/coreos/go-oidc/v3/oidc"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"

	"github.com/canonical/authorization-service/internal/repository"
)

type stubOIDCVerifier struct {
	wantToken string
	err       error
}

func (s *stubOIDCVerifier) Verify(_ context.Context, token string) (*oidc.IDToken, error) {
	if s.err != nil {
		return nil, s.err
	}
	if s.wantToken != "" && token != s.wantToken {
		return nil, errors.New("unexpected token")
	}
	return &oidc.IDToken{}, nil
}

func ctxWithAuth(headerVal string) context.Context {
	return metadata.NewIncomingContext(context.Background(), metadata.Pairs("authorization", headerVal))
}

func TestGRPCIngestionServer_IngestPermissionUpdate(t *testing.T) {
	t.Run("valid token and valid envelope persists and returns OK", func(t *testing.T) {
		repo := &fakeRepo{}
		ing := newIngestor(t, repo, nil)
		srv := NewGRPCIngestionServer(&stubOIDCVerifier{wantToken: "valid-jwt"}, ing, testLogger())

		resp, err := srv.IngestPermissionUpdate(ctxWithAuth("Bearer valid-jwt"), validEnvelope())
		require.NoError(t, err)
		require.NotNil(t, resp)
		require.Len(t, repo.inserted, 1)
		assert.Equal(t, "payments", repo.inserted[0].Service)
	})

	t.Run("duplicate envelope is idempotent and returns OK", func(t *testing.T) {
		repo := &fakeRepo{err: repository.ErrDuplicate}
		ing := newIngestor(t, repo, nil)
		srv := NewGRPCIngestionServer(&stubOIDCVerifier{wantToken: "valid-jwt"}, ing, testLogger())

		resp, err := srv.IngestPermissionUpdate(ctxWithAuth("Bearer valid-jwt"), validEnvelope())
		require.NoError(t, err)
		require.NotNil(t, resp)
	})

	t.Run("nil verifier returns Unauthenticated", func(t *testing.T) {
		repo := &fakeRepo{}
		ing := newIngestor(t, repo, nil)
		srv := NewGRPCIngestionServer(nil, ing, testLogger())

		_, err := srv.IngestPermissionUpdate(ctxWithAuth("Bearer valid-jwt"), validEnvelope())
		require.Error(t, err)
		assert.Equal(t, codes.Unauthenticated, status.Code(err))
		assert.Empty(t, repo.inserted)
	})

	t.Run("missing metadata returns Unauthenticated", func(t *testing.T) {
		repo := &fakeRepo{}
		ing := newIngestor(t, repo, nil)
		srv := NewGRPCIngestionServer(&stubOIDCVerifier{}, ing, testLogger())

		_, err := srv.IngestPermissionUpdate(context.Background(), validEnvelope())
		require.Error(t, err)
		assert.Equal(t, codes.Unauthenticated, status.Code(err))
		assert.Empty(t, repo.inserted)
	})

	t.Run("missing authorization header returns Unauthenticated", func(t *testing.T) {
		repo := &fakeRepo{}
		ing := newIngestor(t, repo, nil)
		srv := NewGRPCIngestionServer(&stubOIDCVerifier{}, ing, testLogger())

		ctx := metadata.NewIncomingContext(context.Background(), metadata.Pairs("x-request-id", "req-1"))
		_, err := srv.IngestPermissionUpdate(ctx, validEnvelope())
		require.Error(t, err)
		assert.Equal(t, codes.Unauthenticated, status.Code(err))
		assert.Empty(t, repo.inserted)
	})

	t.Run("non-Bearer scheme returns Unauthenticated", func(t *testing.T) {
		repo := &fakeRepo{}
		ing := newIngestor(t, repo, nil)
		srv := NewGRPCIngestionServer(&stubOIDCVerifier{}, ing, testLogger())

		_, err := srv.IngestPermissionUpdate(ctxWithAuth("Basic dXNlcjpwYXNz"), validEnvelope())
		require.Error(t, err)
		assert.Equal(t, codes.Unauthenticated, status.Code(err))
		assert.Empty(t, repo.inserted)
	})

	t.Run("invalid JWT signature returns Unauthenticated", func(t *testing.T) {
		repo := &fakeRepo{}
		ing := newIngestor(t, repo, nil)
		srv := NewGRPCIngestionServer(&stubOIDCVerifier{err: errors.New("signature invalid")}, ing, testLogger())

		_, err := srv.IngestPermissionUpdate(ctxWithAuth("Bearer bad-jwt"), validEnvelope())
		require.Error(t, err)
		assert.Equal(t, codes.Unauthenticated, status.Code(err))
		assert.Empty(t, repo.inserted)
	})

	t.Run("unregistered service returns InvalidArgument", func(t *testing.T) {
		repo := &fakeRepo{}
		ing := newIngestor(t, repo, nil)
		srv := NewGRPCIngestionServer(&stubOIDCVerifier{wantToken: "valid-jwt"}, ing, testLogger())

		env := validEnvelope()
		env.Service = "unknown-svc"

		_, err := srv.IngestPermissionUpdate(ctxWithAuth("Bearer valid-jwt"), env)
		require.Error(t, err)
		assert.Equal(t, codes.InvalidArgument, status.Code(err))
		assert.Empty(t, repo.inserted)
	})

	t.Run("malformed envelope returns InvalidArgument", func(t *testing.T) {
		repo := &fakeRepo{}
		ing := newIngestor(t, repo, nil)
		srv := NewGRPCIngestionServer(&stubOIDCVerifier{wantToken: "valid-jwt"}, ing, testLogger())

		env := validEnvelope()
		env.Operations = nil

		_, err := srv.IngestPermissionUpdate(ctxWithAuth("Bearer valid-jwt"), env)
		require.Error(t, err)
		assert.Equal(t, codes.InvalidArgument, status.Code(err))
		assert.Empty(t, repo.inserted)
	})

	t.Run("transient repository error returns Internal", func(t *testing.T) {
		repo := &fakeRepo{err: errors.New("db connection reset")}
		ing := newIngestor(t, repo, nil)
		srv := NewGRPCIngestionServer(&stubOIDCVerifier{wantToken: "valid-jwt"}, ing, testLogger())

		_, err := srv.IngestPermissionUpdate(ctxWithAuth("Bearer valid-jwt"), validEnvelope())
		require.Error(t, err)
		assert.Equal(t, codes.Internal, status.Code(err))
	})
}
