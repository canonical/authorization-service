// Copyright 2026 Canonical Ltd.
// SPDX-License-Identifier: AGPL-3.0-only

package logging

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestRequestIDRoundTrip(t *testing.T) {
	ctx := ContextWithRequestID(context.Background(), "req-123")
	assert.Equal(t, "req-123", RequestIDFromContext(ctx))
}

func TestRequestIDFromContext_Unset(t *testing.T) {
	assert.Equal(t, "", RequestIDFromContext(context.Background()))
}
