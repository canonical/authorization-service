// Copyright 2026 Canonical Ltd.
// SPDX-License-Identifier: AGPL-3.0-only

package testutil

import (
	"context"

	"github.com/testcontainers/testcontainers-go"
)

// StopContainer terminates a test container.
func StopContainer(ctx context.Context, container testcontainers.Container) error {
	return container.Terminate(ctx)
}
