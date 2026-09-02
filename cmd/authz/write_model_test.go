// Copyright 2026 Canonical Ltd.
// SPDX-License-Identifier: AGPL-3.0-only

package authz

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/canonical/authorization-service/config"
)

func TestWriteAuthorizationModel_WithApiKey(t *testing.T) {
	apiKey := "test-secret-api-key"
	receivedAuthHeader := ""

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedAuthHeader = r.Header.Get("Authorization")
		if receivedAuthHeader != "Bearer "+apiKey {
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte(`{"code":"bearer_token_missing","message":"missing bearer token"}`))
			return
		}

		// Return success response for WriteAuthorizationModel
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"authorization_model_id":"01GP1254CHWJC1MNGVB0WDG1T0"}`))
	}))
	defer server.Close()

	cfg := &config.Config{
		OpenFGA: &config.OpenFGAConfig{
			Address: server.URL,
			ApiKey:  apiKey,
			Timeout: 5 * time.Second,
		},
	}

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	err := WriteAuthorizationModel(context.Background(), "01GP1254CHWJC1MNGVB0WDG1T0", cfg, logger)

	require.NoError(t, err)
	assert.Equal(t, "Bearer "+apiKey, receivedAuthHeader)
}

func TestWriteAuthorizationModel_WithoutApiKey(t *testing.T) {
	receivedAuthHeader := ""

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedAuthHeader = r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"authorization_model_id":"01GP1254CHWJC1MNGVB0WDG1T0"}`))
	}))
	defer server.Close()

	cfg := &config.Config{
		OpenFGA: &config.OpenFGAConfig{
			Address: server.URL,
			ApiKey:  "",
			Timeout: 5 * time.Second,
		},
	}

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	err := WriteAuthorizationModel(context.Background(), "01GP1254CHWJC1MNGVB0WDG1T0", cfg, logger)

	require.NoError(t, err)
	assert.Empty(t, receivedAuthHeader)
}
