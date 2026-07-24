//go:build integration

package integration

import (
	"context"
	"net"
	"testing"
	"time"

	"github.com/openfga/go-sdk/client"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"

	"github.com/canonical/authorization-service/cmd/authz"
	"github.com/canonical/authorization-service/config"
)

func TestWriteModel_Integration(t *testing.T) {
	ctx := context.Background()

	// 1. Start a temporary OpenFGA container (in-memory, fast, and isolated)
	req := testcontainers.ContainerRequest{
		Image:        "openfga/openfga:v1.14.1",
		ExposedPorts: []string{"8080/tcp", "8081/tcp"},
		Cmd:          []string{"run"},
		WaitingFor:   wait.ForHTTP("/healthz").WithPort("8080/tcp").WithStartupTimeout(30 * time.Second),
	}

	container, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: req,
		Started:          true,
	})
	if err != nil {
		t.Fatalf("failed to start OpenFGA container: %v", err)
	}
	defer func() {
		_ = container.Terminate(ctx)
	}()

	// Get the mapped HTTP port from the test container
	mappedPort, err := container.MappedPort(ctx, "8080/tcp")
	if err != nil {
		t.Fatalf("failed to get mapped port: %v", err)
	}
	host, err := container.Host(ctx)
	if err != nil {
		t.Fatalf("failed to get container host: %v", err)
	}
	openfgaAddr := net.JoinHostPort(host, mappedPort.Port())

	// 2. Initialize OpenFGA SDK Client to create a temporary test store
	clientConfig := &client.ClientConfiguration{
		ApiUrl: "http://" + openfgaAddr,
	}
	fgaClient, err := client.NewSdkClient(clientConfig)
	if err != nil {
		t.Fatalf("failed to create OpenFGA SDK client: %v", err)
	}

	store, err := fgaClient.CreateStore(ctx).Body(client.ClientCreateStoreRequest{
		Name: "integration-test-store",
	}).Execute()
	if err != nil {
		t.Fatalf("failed to create OpenFGA test store: %v", err)
	}
	storeID := store.GetId()

	// 3. Prepare config with nested OpenFGAConfig initialized
	cfg := &config.Config{
		OpenFGA: &config.OpenFGAConfig{
			Address: "http://" + openfgaAddr,
			Timeout: 10 * time.Second,
		},
	}

	// 4. Upload the compiled modular model by invoking the exported WriteAuthorizationModel
	err = authz.WriteAuthorizationModel(ctx, storeID, cfg, testLogger)
	if err != nil {
		t.Fatalf("failed to write modular authorization model: %v", err)
	}

	// 5. Verification: Read back the written model from OpenFGA and assert types & conditions
	fgaClient.SetStoreId(storeID)
	modelResp, err := fgaClient.ReadLatestAuthorizationModel(ctx).Execute()
	if err != nil {
		t.Fatalf("failed to read latest authorization model: %v", err)
	}

	model := modelResp.GetAuthorizationModel()

	// Check core and federated types
	expectedTypes := map[string]bool{
		"user":          false,
		"group":         false,
		"role":          false,
		"generic-asset": false,
		"domainAdmin":   false,
		"smallRole":     false,
		"testGroup":     false,
	}

	for _, td := range model.GetTypeDefinitions() {
		if _, ok := expectedTypes[td.GetType()]; ok {
			expectedTypes[td.GetType()] = true
		}
	}

	for typeName, found := range expectedTypes {
		if !found {
			t.Errorf("expected type %q not found in written authorization model", typeName)
		}
	}

	// Check conditions
	conditions := model.GetConditions()
	if len(conditions) == 0 {
		t.Error("expected conditions to be present in the compiled model")
	} else {
		foundTenantMatch := false
		for condName := range conditions {
			if condName == "tenant_match" {
				foundTenantMatch = true
				break
			}
		}
		if !foundTenantMatch {
			t.Error("expected condition 'tenant_match' not found in written model")
		}
	}
}
