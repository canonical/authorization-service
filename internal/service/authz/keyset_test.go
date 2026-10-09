// Copyright 2026 Canonical Ltd.
// SPDX-License-Identifier: AGPL-3.0-only

package authz

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	jose "github.com/go-jose/go-jose/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func generateTestKey(t *testing.T, keyID string) (*rsa.PrivateKey, jose.JSONWebKey) {
	t.Helper()
	priv, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)

	jwk := jose.JSONWebKey{
		Key:       &priv.PublicKey,
		KeyID:     keyID,
		Algorithm: string(jose.RS256),
		Use:       "sig",
	}
	return priv, jwk
}

func signJWT(t *testing.T, priv *rsa.PrivateKey, keyID string, payload []byte) string {
	t.Helper()
	signer, err := jose.NewSigner(
		jose.SigningKey{Algorithm: jose.RS256, Key: priv},
		(&jose.SignerOptions{}).WithType("JWT").WithHeader("kid", keyID),
	)
	require.NoError(t, err)

	signed, err := signer.Sign(payload)
	require.NoError(t, err)

	serialized, err := signed.CompactSerialize()
	require.NoError(t, err)
	return serialized
}

func TestPreemptingKeySet_WarmUpAndVerify(t *testing.T) {
	priv1, jwk1 := generateTestKey(t, "key-1")

	var fetchCount atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fetchCount.Add(1)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(jose.JSONWebKeySet{
			Keys: []jose.JSONWebKey{jwk1},
		})
	}))
	defer server.Close()

	ctx := context.Background()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	// 1. Initialization eagerly warms the cache
	ks := NewPreemptingKeySet(ctx, server.URL, server.Client(), logger)
	require.Equal(t, int32(1), fetchCount.Load(), "should fetch JWKS during NewPreemptingKeySet")
	require.Len(t, ks.Keys(), 1)
	assert.Equal(t, "key-1", ks.Keys()[0].KeyID)

	// 2. Verification using cached key does not trigger additional fetch
	payload := []byte(`{"sub":"client-1","iss":"test"}`)
	token := signJWT(t, priv1, "key-1", payload)

	verifiedPayload, err := ks.VerifySignature(ctx, token)
	require.NoError(t, err)
	assert.Equal(t, payload, verifiedPayload)
	assert.Equal(t, int32(1), fetchCount.Load(), "cached verification should not trigger a remote fetch")
}

func TestPreemptingKeySet_KeyRotation(t *testing.T) {
	_, jwk1 := generateTestKey(t, "key-1")
	priv2, jwk2 := generateTestKey(t, "key-2")

	var currentKeys atomic.Value
	currentKeys.Store([]jose.JSONWebKey{jwk1})

	var fetchCount atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fetchCount.Add(1)
		w.Header().Set("Content-Type", "application/json")
		keys := currentKeys.Load().([]jose.JSONWebKey)
		_ = json.NewEncoder(w).Encode(jose.JSONWebKeySet{
			Keys: keys,
		})
	}))
	defer server.Close()

	ctx := context.Background()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	// Initial warm-up with key-1
	ks := NewPreemptingKeySet(ctx, server.URL, server.Client(), logger)
	require.Equal(t, int32(1), fetchCount.Load())

	// Rotate key on server to include key-2
	currentKeys.Store([]jose.JSONWebKey{jwk1, jwk2})

	// Sign token with key-2 (not in current cache)
	payload := []byte(`{"sub":"client-2","iss":"test"}`)
	token := signJWT(t, priv2, "key-2", payload)

	// Verification should miss cache, trigger refresh, and succeed
	verifiedPayload, err := ks.VerifySignature(ctx, token)
	require.NoError(t, err)
	assert.Equal(t, payload, verifiedPayload)
	assert.Equal(t, int32(2), fetchCount.Load(), "signature failure with unknown key should trigger JWKS refresh")

	// Next verification with key-2 should use cache
	_, err = ks.VerifySignature(ctx, token)
	require.NoError(t, err)
	assert.Equal(t, int32(2), fetchCount.Load(), "subsequent verification should hit cache")
}

func TestPreemptingKeySet_InvalidToken(t *testing.T) {
	_, jwk1 := generateTestKey(t, "key-1")
	unknownPriv, _ := generateTestKey(t, "key-unknown")

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(jose.JSONWebKeySet{
			Keys: []jose.JSONWebKey{jwk1},
		})
	}))
	defer server.Close()

	ctx := context.Background()
	ks := NewPreemptingKeySet(ctx, server.URL, server.Client(), nil)

	// Token signed with unknown key that never appears in server JWKS
	token := signJWT(t, unknownPriv, "key-unknown", []byte(`{"sub":"bad"}`))
	_, err := ks.VerifySignature(ctx, token)
	assert.Error(t, err)

	// Malformed token string
	_, err = ks.VerifySignature(ctx, "not.a.valid.jwt")
	assert.Error(t, err)
}

func TestPreemptingKeySet_StartupFailureHandled(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "internal server error", http.StatusInternalServerError)
	}))
	defer server.Close()

	ctx := context.Background()
	// Should not panic, but log a warning and return a keySet with 0 keys
	ks := NewPreemptingKeySet(ctx, server.URL, server.Client(), nil)
	assert.Empty(t, ks.Keys())
}
