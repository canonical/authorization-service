// Copyright 2026 Canonical Ltd.
// SPDX-License-Identifier: AGPL-3.0-only

package authz

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBuildCheckRequest_WithCookie(t *testing.T) {
	req, err := buildCheckRequest("GET", "/api/v1/items", "api.example.com", "session=xyz", "")
	require.NoError(t, err)

	httpReq := req.GetAttributes().GetRequest().GetHttp()
	assert.Equal(t, "GET", httpReq.GetMethod())
	assert.Equal(t, "/api/v1/items", httpReq.GetPath())
	assert.Equal(t, "api.example.com", httpReq.GetHost())
	assert.Equal(t, "session=xyz", httpReq.GetHeaders()["cookie"])
	assert.NotContains(t, httpReq.GetHeaders(), "authorization")
}

func TestBuildCheckRequest_WithToken(t *testing.T) {
	req, err := buildCheckRequest("POST", "/api/v1/items", "api.example.com", "", "m2m-jwt-token")
	require.NoError(t, err)

	httpReq := req.GetAttributes().GetRequest().GetHttp()
	assert.Equal(t, "POST", httpReq.GetMethod())
	assert.Equal(t, "/api/v1/items", httpReq.GetPath())
	assert.Equal(t, "Bearer m2m-jwt-token", httpReq.GetHeaders()["authorization"])
	assert.NotContains(t, httpReq.GetHeaders(), "cookie")
}

func TestBuildCheckRequest_MissingCookieAndToken(t *testing.T) {
	req, err := buildCheckRequest("GET", "/api/v1/items", "api.example.com", "", "")
	assert.Nil(t, req)
	require.Error(t, err)
	assert.ErrorContains(t, err, "either --cookie or --token is required")
}
