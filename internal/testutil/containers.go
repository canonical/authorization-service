package testutil

import (
	"context"
	"testing"

	"github.com/canonical/authorization-service/internal/integrations/nats"
	natslib "github.com/nats-io/nats.go"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
)

// StartNATSContainer starts a NATS container for integration tests
func StartNATSContainer(ctx context.Context, t *testing.T) (testcontainers.Container, *nats.Client) {
	req := testcontainers.ContainerRequest{
		Image:        "nats:latest",
		ExposedPorts: []string{"4222/tcp"},
		Cmd:          []string{"-js", "-m", "8222"},
		WaitingFor:   wait.ForLog("Server is ready"),
	}

	container, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: req,
		Started:          true,
	})
	if err != nil {
		t.Fatalf("Failed to start NATS container: %v", err)
	}

	host, err := container.Host(ctx)
	if err != nil {
		container.Terminate(ctx)
		t.Fatalf("Failed to get container host: %v", err)
	}

	port, err := container.MappedPort(ctx, "4222")
	if err != nil {
		container.Terminate(ctx)
		t.Fatalf("Failed to get mapped port: %v", err)
	}

	url := "nats://" + host + ":" + port.Port()

	// Create NATS client
	client, err := nats.NewClient(nats.Config{
		URL:             url,
		ClientID:        "test-client",
		EnableJetStream: true,
		StreamName:      "TEST",
	}, NewTestLogger(t))
	if err != nil {
		container.Terminate(ctx)
		t.Fatalf("Failed to create NATS client: %v", err)
	}

	return container, client
}

// StopContainer stops a test container
func StopContainer(ctx context.Context, container testcontainers.Container) error {
	return container.Terminate(ctx)
}

// MockNATSClient is a mock implementation of nats.Client for testing
type MockNATSClient struct {
	PublishFunc     func(ctx context.Context, subject string, data []byte) error
	SubscribeFunc   func(ctx context.Context, subject string, handler func(msg *natslib.Msg) error) error
	CloseFunc       func() error
	IsConnectedFunc func() bool
}

// Publish implements nats.Client
func (m *MockNATSClient) Publish(ctx context.Context, subject string, data []byte) error {
	if m.PublishFunc != nil {
		return m.PublishFunc(ctx, subject, data)
	}
	return nil
}

// Subscribe implements nats.Client
func (m *MockNATSClient) Subscribe(ctx context.Context, subject string, handler func(msg *natslib.Msg) error) error {
	if m.SubscribeFunc != nil {
		return m.SubscribeFunc(ctx, subject, handler)
	}
	return nil
}

// Close implements nats.Client
func (m *MockNATSClient) Close() error {
	if m.CloseFunc != nil {
		return m.CloseFunc()
	}
	return nil
}

// IsConnected implements nats.Client
func (m *MockNATSClient) IsConnected() bool {
	if m.IsConnectedFunc != nil {
		return m.IsConnectedFunc()
	}
	return true
}
