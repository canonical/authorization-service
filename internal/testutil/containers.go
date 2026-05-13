package testutil

import (
	"context"

	"github.com/testcontainers/testcontainers-go"
)

// StopContainer terminates a test container.
func StopContainer(ctx context.Context, container testcontainers.Container) error {
	return container.Terminate(ctx)
}
