//go:build integration

package check

import (
	"context"
	"net"
	"testing"
	"time"

	openfga "github.com/openfga/go-sdk"
	"github.com/openfga/go-sdk/client"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"

	"github.com/canonical/authorization-service/cmd/authz"
	"github.com/canonical/authorization-service/config"
	"github.com/canonical/authorization-service/tests/integration/suite"
)

/*
| Subtest | Tuple Context | Check-Time Context | Expected | Result | Note |
| :--- | :--- | :--- | :---: | :---: | :--- |
| **Tuple 1: Matching tenant (Canonical)** | `{"tenant_enabled": true, "tenant": "Canonical"}` | `{"user_tenant": "Canonical"}` | **Allow** | **PASS** | Access permitted when tenants match |
| **Tuple 1: Mismatched tenant (Ubuntu)** | `{"tenant_enabled": true, "tenant": "Canonical"}` | `{"user_tenant": "Ubuntu"}` | **Deny** | **PASS** | Access denied when tenants mismatch |
| **Tuple 1: Empty user tenant** | `{"tenant_enabled": true, "tenant": "Canonical"}` | `{"user_tenant": ""}` | **Deny** | **PASS** | Access denied when user tenant is missing |
| **Tuple 2: Global resource - User canonical tenant** | `{"tenant_enabled": true, "tenant": ""}` | `{"user_tenant": "Canonical"}` | **Allow** | **PASS** | Global resource is accessible |
| **Tuple 2: Global resource - User ubuntu tenant** | `{"tenant_enabled": true, "tenant": ""}` | `{"user_tenant": "Ubuntu"}` | **Allow** | **PASS** | Global resource is accessible |
| **Tuple 2: Global resource - User empty tenant** | `{"tenant_enabled": true, "tenant": ""}` | `{"user_tenant": ""}` | **Allow** | **PASS** | Global resource is accessible |
| **Tuple 3: Tenancy disabled - User canonical tenant** | `{"tenant_enabled": false, "tenant": "Canonical"}` | `{"user_tenant": "Canonical"}` | **Allow** | **PASS** | Allowed when tenancy is bypassed |
| **Tuple 3: Tenancy disabled - User ubuntu tenant** | `{"tenant_enabled": false, "tenant": "Canonical"}` | `{"user_tenant": "Ubuntu"}` | **Allow** | **PASS** | Allowed when tenancy is bypassed |
| **Tuple 4: Dynamic tenancy active - Match** | `{"tenant": "Canonical"}` | `{"tenant_enabled": true, "user_tenant": "Canonical"}` | **Allow** | **PASS** | Dynamic context evaluation |
| **Tuple 4: Dynamic tenancy active - Mismatch** | `{"tenant": "Canonical"}` | `{"tenant_enabled": true, "user_tenant": "Ubuntu"}` | **Deny** | **PASS** | Dynamic context evaluation |
| **Tuple 4: Dynamic tenancy disabled - Mismatch** | `{"tenant": "Canonical"}` | `{"tenant_enabled": false, "user_tenant": "Ubuntu"}` | **Allow** | **PASS** | Dynamic context evaluation |
*/

func TestTenantMatch_Integration(t *testing.T) {
	ctx := context.Background()

	// 1. Start a temporary OpenFGA container
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
		Name: "tenant-match-test-store",
	}).Execute()
	if err != nil {
		t.Fatalf("failed to create OpenFGA test store: %v", err)
	}
	storeID := store.GetId()

	// 3. Prepare config with nested OpenFGAConfig
	cfg := &config.Config{
		OpenFGA: &config.OpenFGAConfig{
			Address: "http://" + openfgaAddr,
			Timeout: 10 * time.Second,
		},
	}

	// 4. Upload the compiled modular model
	err = authz.WriteAuthorizationModel(ctx, storeID, cfg, suite.TestLogger)
	if err != nil {
		t.Fatalf("failed to write modular authorization model: %v", err)
	}

	fgaClient.SetStoreId(storeID)

	// 5. Write test tuples
	tuples := []client.ClientTupleKey{
		// Tuple 1: Tenant-specific under tenancy (Canonical)
		{
			User:     "user:alice",
			Relation: "member",
			Object:   "group:g1",
			Condition: &openfga.RelationshipCondition{
				Name: "tenant_match",
				Context: &map[string]interface{}{
					"tenant_enabled": true,
					"tenant":         "Canonical",
				},
			},
		},
		// Tuple 2: Global resource (tenant is empty string)
		{
			User:     "user:bob",
			Relation: "member",
			Object:   "group:g2",
			Condition: &openfga.RelationshipCondition{
				Name: "tenant_match",
				Context: &map[string]interface{}{
					"tenant_enabled": true,
					"tenant":         "",
				},
			},
		},
		// Tuple 3: Tenancy disabled
		{
			User:     "user:charlie",
			Relation: "member",
			Object:   "group:g3",
			Condition: &openfga.RelationshipCondition{
				Name: "tenant_match",
				Context: &map[string]interface{}{
					"tenant_enabled": false,
					"tenant":         "Canonical",
				},
			},
		},
		// Tuple 4: Tenant-specific but tenancy configuration is dynamic at check-time
		{
			User:     "user:dave",
			Relation: "member",
			Object:   "group:g4",
			Condition: &openfga.RelationshipCondition{
				Name: "tenant_match",
				Context: &map[string]interface{}{
					"tenant": "Canonical",
				},
			},
		},
	}

	_, err = fgaClient.Write(ctx).Body(client.ClientWriteRequest{
		Writes: tuples,
	}).Execute()
	if err != nil {
		t.Fatalf("failed to write test tuples: %v", err)
	}

	// 6. Define check cases to assert
	testCases := []struct {
		name         string
		user         string
		relation     string
		object       string
		checkContext map[string]interface{}
		expectAllow  bool
	}{
		// --- Cases for Tuple 1 (tenant_enabled = true, tenant = "Canonical") ---
		{
			name:         "Tuple 1: Matching tenant (Canonical)",
			user:         "user:alice",
			relation:     "member",
			object:       "group:g1",
			checkContext: map[string]interface{}{"user_tenant": "Canonical"},
			expectAllow:  true,
		},
		{
			name:         "Tuple 1: Mismatched tenant (Ubuntu)",
			user:         "user:alice",
			relation:     "member",
			object:       "group:g1",
			checkContext: map[string]interface{}{"user_tenant": "Ubuntu"},
			expectAllow:  false,
		},
		{
			name:         "Tuple 1: Empty user tenant",
			user:         "user:alice",
			relation:     "member",
			object:       "group:g1",
			checkContext: map[string]interface{}{"user_tenant": ""},
			expectAllow:  false,
		},

		// --- Cases for Tuple 2 (tenant_enabled = true, tenant = "") ---
		{
			name:         "Tuple 2: Global resource - User canonical tenant",
			user:         "user:bob",
			relation:     "member",
			object:       "group:g2",
			checkContext: map[string]interface{}{"user_tenant": "Canonical"},
			expectAllow:  true,
		},
		{
			name:         "Tuple 2: Global resource - User ubuntu tenant",
			user:         "user:bob",
			relation:     "member",
			object:       "group:g2",
			checkContext: map[string]interface{}{"user_tenant": "Ubuntu"},
			expectAllow:  true,
		},
		{
			name:         "Tuple 2: Global resource - User empty tenant",
			user:         "user:bob",
			relation:     "member",
			object:       "group:g2",
			checkContext: map[string]interface{}{"user_tenant": ""},
			expectAllow:  true,
		},

		// --- Cases for Tuple 3 (tenant_enabled = false, tenant = "Canonical") ---
		{
			name:         "Tuple 3: Tenancy disabled - User canonical tenant",
			user:         "user:charlie",
			relation:     "member",
			object:       "group:g3",
			checkContext: map[string]interface{}{"user_tenant": "Canonical"},
			expectAllow:  true,
		},
		{
			name:         "Tuple 3: Tenancy disabled - User ubuntu tenant (mismatch but allowed)",
			user:         "user:charlie",
			relation:     "member",
			object:       "group:g3",
			checkContext: map[string]interface{}{"user_tenant": "Ubuntu"},
			expectAllow:  true,
		},

		// --- Cases for Tuple 4 (dynamic tenant_enabled and user_tenant at check-time) ---
		{
			name:     "Tuple 4: Dynamic tenancy active - Matching tenant",
			user:     "user:dave",
			relation: "member",
			object:   "group:g4",
			checkContext: map[string]interface{}{
				"tenant_enabled": true,
				"user_tenant":    "Canonical",
			},
			expectAllow: true,
		},
		{
			name:     "Tuple 4: Dynamic tenancy active - Mismatched tenant (Ubuntu)",
			user:     "user:dave",
			relation: "member",
			object:   "group:g4",
			checkContext: map[string]interface{}{
				"tenant_enabled": true,
				"user_tenant":    "Ubuntu",
			},
			expectAllow: false,
		},
		{
			name:     "Tuple 4: Dynamic tenancy disabled - Mismatched tenant (Ubuntu)",
			user:     "user:dave",
			relation: "member",
			object:   "group:g4",
			checkContext: map[string]interface{}{
				"tenant_enabled": false,
				"user_tenant":    "Ubuntu",
			},
			expectAllow: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			checkReq := client.ClientCheckRequest{
				User:     tc.user,
				Relation: tc.relation,
				Object:   tc.object,
				Context:  &tc.checkContext,
			}
			resp, err := fgaClient.Check(ctx).Body(checkReq).Execute()
			if err != nil {
				t.Fatalf("check failed: %v", err)
			}
			allowed := resp.GetAllowed()
			if allowed != tc.expectAllow {
				t.Errorf("expected allowed=%t, got %t", tc.expectAllow, allowed)
			}
		})
	}
}
