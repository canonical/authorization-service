// Copyright 2026 Canonical Ltd.
// SPDX-License-Identifier: AGPL-3.0-only

package authz

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"
	jose "github.com/go-jose/go-jose/v4"
)

var allAlgs = []jose.SignatureAlgorithm{
	jose.RS256, jose.RS384, jose.RS512,
	jose.ES256, jose.ES384, jose.ES512,
	jose.EdDSA,
}

var _ oidc.KeySet = (*PreemptingKeySet)(nil)

// PreemptingKeySet implements oidc.KeySet and warms the JWKS cache on startup.
type PreemptingKeySet struct {
	jwksURL    string
	httpClient *http.Client
	logger     *slog.Logger

	mu   sync.RWMutex
	keys []jose.JSONWebKey
}

// NewPreemptingKeySet creates a PreemptingKeySet and preemptively fetches the JWKS keys.
func NewPreemptingKeySet(ctx context.Context, jwksURL string, client *http.Client, logger *slog.Logger) *PreemptingKeySet {
	if client == nil {
		client = &http.Client{Timeout: 5 * time.Second}
	}
	if logger == nil {
		logger = slog.Default()
	}

	p := &PreemptingKeySet{
		jwksURL:    jwksURL,
		httpClient: client,
		logger:     logger,
	}

	if err := p.refresh(ctx); err != nil {
		p.logger.Warn("Failed to preemptively warm JWKS cache during startup; will retry on demand", "url", jwksURL, "error", err)
	} else {
		p.mu.RLock()
		count := len(p.keys)
		p.mu.RUnlock()
		p.logger.Info("Successfully pre-fetched and warmed JWKS cache", "url", jwksURL, "keys_count", count)
	}

	return p
}

func (p *PreemptingKeySet) refresh(ctx context.Context) error {
	reqCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, p.jwksURL, nil)
	if err != nil {
		return fmt.Errorf("failed to create JWKS request: %w", err)
	}

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to fetch JWKS: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("JWKS endpoint returned status: %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read JWKS body: %w", err)
	}

	var keySet jose.JSONWebKeySet
	if err := json.Unmarshal(body, &keySet); err != nil {
		return fmt.Errorf("failed to unmarshal JWKS: %w", err)
	}

	p.mu.Lock()
	p.keys = keySet.Keys
	p.mu.Unlock()

	return nil
}

// Keys returns a copy of the currently cached keys (useful for tests/inspection).
func (p *PreemptingKeySet) Keys() []jose.JSONWebKey {
	p.mu.RLock()
	defer p.mu.RUnlock()
	copied := make([]jose.JSONWebKey, len(p.keys))
	copy(copied, p.keys)
	return copied
}

// VerifySignature validates a JWT payload against the cached or refreshed JWKS keys.
func (p *PreemptingKeySet) VerifySignature(ctx context.Context, jwt string) ([]byte, error) {
	jws, err := jose.ParseSigned(jwt, allAlgs)
	if err != nil {
		return nil, fmt.Errorf("malformed jwt: %w", err)
	}

	keyID := ""
	for _, sig := range jws.Signatures {
		keyID = sig.Header.KeyID
		break
	}

	// 1. Try cached keys first
	p.mu.RLock()
	for _, key := range p.keys {
		if keyID == "" || key.KeyID == keyID {
			if payload, err := jws.Verify(&key); err == nil {
				p.mu.RUnlock()
				return payload, nil
			}
		}
	}
	p.mu.RUnlock()

	// 2. If no key matched or verification failed, refresh from remote (key rotation)
	if err := p.refresh(ctx); err != nil {
		return nil, fmt.Errorf("fetching remote keys: %w", err)
	}

	// 3. Retry verification with refreshed keys
	p.mu.RLock()
	defer p.mu.RUnlock()
	for _, key := range p.keys {
		if keyID == "" || key.KeyID == keyID {
			if payload, err := jws.Verify(&key); err == nil {
				return payload, nil
			}
		}
	}

	return nil, errors.New("failed to verify token signature")
}
