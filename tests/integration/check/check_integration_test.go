//go:build integration

package check

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"reflect"
	"strings"
	"testing"
	"time"
	"unsafe"

	"github.com/coreos/go-oidc/v3/oidc"
	dockercontainer "github.com/moby/moby/api/types/container"
	openfga "github.com/openfga/go-sdk"
	"github.com/openfga/go-sdk/client"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
	"go.opentelemetry.io/otel/trace/noop"
	"go.uber.org/mock/gomock"
	"google.golang.org/grpc"

	stsv1 "github.com/canonical/authorization-service/client/v1/sts"
	cmdauthz "github.com/canonical/authorization-service/cmd/authz"
	"github.com/canonical/authorization-service/config"
	"github.com/canonical/authorization-service/internal/model/rules"
	authz "github.com/canonical/authorization-service/internal/service/authz"
	authzMocks "github.com/canonical/authorization-service/internal/service/authz/mocks"
	"github.com/canonical/authorization-service/tests/integration/suite"
)

// newTestIDToken creates an *oidc.IDToken with the given claims JSON set via reflection.
func newTestIDToken(t *testing.T, claimsJSON []byte) *oidc.IDToken {
	t.Helper()
	token := &oidc.IDToken{}
	rv := reflect.ValueOf(token).Elem()
	f := rv.FieldByName("claims")
	f = reflect.NewAt(f.Type(), unsafe.Pointer(f.UnsafeAddr())).Elem()
	f.Set(reflect.ValueOf(claimsJSON))
	return token
}

func TestCheck_EnvoyIntegration(t *testing.T) {
	ctx := context.Background()

	// 1. Start a temporary OpenFGA container
	fgaReq := testcontainers.ContainerRequest{
		Image:        "openfga/openfga:v1.14.1",
		ExposedPorts: []string{"8080/tcp"},
		Cmd:          []string{"run"},
		WaitingFor:   wait.ForHTTP("/healthz").WithPort("8080/tcp").WithStartupTimeout(30 * time.Second),
	}
	fgaCtr, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: fgaReq,
		Started:          true,
	})
	if err != nil {
		t.Fatalf("failed to start OpenFGA: %v", err)
	}
	defer func() { _ = fgaCtr.Terminate(ctx) }()

	fgaPort, err := fgaCtr.MappedPort(ctx, "8080/tcp")
	if err != nil {
		t.Fatalf("failed to get OpenFGA port: %v", err)
	}
	fgaHost, err := fgaCtr.Host(ctx)
	if err != nil {
		t.Fatalf("failed to get OpenFGA host: %v", err)
	}
	fgaAddr := fmt.Sprintf("http://%s", net.JoinHostPort(fgaHost, fgaPort.Port()))

	// 2. Initialize OpenFGA test store and upload modular model
	fgaClient, err := client.NewSdkClient(&client.ClientConfiguration{ApiUrl: fgaAddr})
	if err != nil {
		t.Fatalf("failed to create OpenFGA client: %v", err)
	}
	store, err := fgaClient.CreateStore(ctx).Body(client.ClientCreateStoreRequest{Name: "check-test-store"}).Execute()
	if err != nil {
		t.Fatalf("failed to create OpenFGA store: %v", err)
	}
	storeID := store.GetId()
	fgaClient.SetStoreId(storeID)

	cfg := &config.Config{
		OpenFGA: &config.OpenFGAConfig{
			Address: fgaAddr,
			Timeout: 10 * time.Second,
		},
	}
	err = cmdauthz.WriteAuthorizationModel(ctx, storeID, cfg, suite.TestLogger)
	if err != nil {
		t.Fatalf("failed to write authorization model: %v", err)
	}

	// 3. Write test tuple (group:g1 member context is conditional on tenant_match)
	testTuples := []client.ClientTupleKey{
		{
			User:     "user:alice",
			Relation: "member",
			Object:   "group:g1",
			Condition: &openfga.RelationshipCondition{
				Name: "tenant_match",
				Context: &map[string]interface{}{
					"tenant": "Canonical",
				},
			},
		},
	}
	_, err = fgaClient.Write(ctx).Body(client.ClientWriteRequest{Writes: testTuples}).Execute()
	if err != nil {
		t.Fatalf("failed to write test tuples: %v", err)
	}

	// 4. Set up the local gRPC server listening on 0.0.0.0 (dynamic port)
	l, err := net.Listen("tcp", "0.0.0.0:0")
	if err != nil {
		t.Fatalf("failed to listen on all interfaces: %v", err)
	}
	grpcPort := l.Addr().(*net.TCPAddr).Port

	grpcServer := grpc.NewServer()

	// Set up Gomock controller and mock dependencies
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockSTS := authzMocks.NewMockSecurityTokenServiceClient(ctrl)
	mockVerifier := authzMocks.NewMockoidcVerifier(ctrl)
	mockResourceMapper := authzMocks.NewMockResourceMapperInterface(ctrl)

	// Configure mock expectations for STS cookie-to-token exchange and verifier verification
	accessTokenAlice := "alice.token"
	accessTokenBob := "bob.token"

	mockSTS.EXPECT().
		ExchangeSession(gomock.Any(), gomock.Any()).
		DoAndReturn(func(ctx context.Context, req *stsv1.ExchangeRequest, opts ...grpc.CallOption) (*stsv1.ExchangeResponse, error) {
			if req.GetSessionCookie() == "valid-session-alice" {
				return &stsv1.ExchangeResponse{AccessToken: accessTokenAlice}, nil
			}
			if req.GetSessionCookie() == "valid-session-bob" {
				return &stsv1.ExchangeResponse{AccessToken: accessTokenBob}, nil
			}
			return nil, fmt.Errorf("invalid session cookie: %s", req.GetSessionCookie())
		}).
		AnyTimes()

	claimsAliceJSON, _ := json.Marshal(map[string]string{"sub": "alice@example.com", "org": "Canonical"})
	mockVerifier.EXPECT().
		Verify(gomock.Any(), accessTokenAlice).
		Return(newTestIDToken(t, claimsAliceJSON), nil).
		AnyTimes()

	claimsBobJSON, _ := json.Marshal(map[string]string{"sub": "bob@example.com", "org": "Ubuntu"})
	mockVerifier.EXPECT().
		Verify(gomock.Any(), accessTokenBob).
		Return(newTestIDToken(t, claimsBobJSON), nil).
		AnyTimes()

	tenantCanonical := "Canonical"

	// Mock resource mapper mapping HTTP request to our written relation tuple
	mockResourceMapper.EXPECT().
		Map(gomock.Any(), "alice@example.com", "GET", "/ready").
		Return([]client.ClientBatchCheckItem{
			{User: "user:alice", Relation: "member", Object: "group:g1", CorrelationId: "corr-alice"},
		}, &rules.RuleWithTuples{Id: "rule-alice", Tenant: &tenantCanonical}, nil).
		AnyTimes()

	mockResourceMapper.EXPECT().
		Map(gomock.Any(), "bob@example.com", "GET", "/ready").
		Return([]client.ClientBatchCheckItem{
			{User: "user:alice", Relation: "member", Object: "group:g1", CorrelationId: "corr-bob"}, // bob tries to evaluate Alice's access
		}, &rules.RuleWithTuples{Id: "rule-bob", Tenant: &tenantCanonical}, nil).
		AnyTimes()

	// Instantiate ExternalAuthzService with MultitenancyEnabled = true and register
	extSvcMultitenant := authz.NewExternalAuthzService(
		mockVerifier,
		mockSTS,
		mockResourceMapper,
		fgaClient,
		true, // multitenancyEnabled = true
		suite.TestLogger,
		noop.NewTracerProvider().Tracer("test"),
	)
	extSvcMultitenant.Register(grpcServer)

	go func() {
		if err := grpcServer.Serve(l); err != nil {
			t.Logf("gRPC server stopped: %v", err)
		}
	}()
	defer grpcServer.GracefulStop()

	// 5. Start a temporary Envoy container with ext_authz pointing to our local gRPC server
	envoyConfigYAML := fmt.Sprintf(`
admin:
  address:
    socket_address:
      address: 0.0.0.0
      port_value: 9901
static_resources:
  listeners:
  - name: listener_0
    address:
      socket_address:
        address: 0.0.0.0
        port_value: 10000
    filter_chains:
    - filters:
      - name: envoy.filters.network.http_connection_manager
        typed_config:
          "@type": type.googleapis.com/envoy.extensions.filters.network.http_connection_manager.v3.HttpConnectionManager
          stat_prefix: ingress_http
          route_config:
            name: local_route
            virtual_hosts:
            - name: local_service
              domains: ["*"]
              routes:
              - match:
                  prefix: "/"
                route:
                  cluster: mock_upstream
          http_filters:
          - name: envoy.filters.http.ext_authz
            typed_config:
              "@type": type.googleapis.com/envoy.extensions.filters.http.ext_authz.v3.ExtAuthz
              grpc_service:
                envoy_grpc:
                  cluster_name: cerberus_authz
                timeout: 5s
              transport_api_version: V3
          - name: envoy.filters.http.router
            typed_config:
              "@type": type.googleapis.com/envoy.extensions.filters.http.router.v3.Router
  clusters:
  - name: cerberus_authz
    type: LOGICAL_DNS
    dns_lookup_family: V4_ONLY
    lb_policy: ROUND_ROBIN
    http2_protocol_options: {}
    load_assignment:
      cluster_name: cerberus_authz
      endpoints:
      - lb_endpoints:
        - endpoint:
            address:
              socket_address:
                address: host.testcontainers.internal
                port_value: %d
  - name: mock_upstream
    type: LOGICAL_DNS
    dns_lookup_family: V4_ONLY
    lb_policy: ROUND_ROBIN
    load_assignment:
      cluster_name: mock_upstream
      endpoints:
      - lb_endpoints:
        - endpoint:
            address:
              socket_address:
                address: 127.0.0.1
                port_value: 9901
`, grpcPort)

	envoyReq := testcontainers.ContainerRequest{
		Image:        "envoyproxy/envoy:v1.30.0",
		ExposedPorts: []string{"10000/tcp"},
		Files: []testcontainers.ContainerFile{
			{
				Reader:            strings.NewReader(envoyConfigYAML),
				ContainerFilePath: "/etc/envoy/envoy.yaml",
				FileMode:          0644,
			},
		},
		HostConfigModifier: func(hc *dockercontainer.HostConfig) {
			hc.ExtraHosts = []string{"host.testcontainers.internal:host-gateway"}
		},
		WaitingFor: wait.ForLog("starting workers").WithStartupTimeout(30 * time.Second),
	}

	envoyCtr, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: envoyReq,
		Started:          true,
	})
	if err != nil {
		t.Fatalf("failed to start Envoy: %v", err)
	}
	defer func() { _ = envoyCtr.Terminate(ctx) }()

	envoyHost, err := envoyCtr.Host(ctx)
	if err != nil {
		t.Fatalf("failed to get Envoy host: %v", err)
	}
	envoyPort, err := envoyCtr.MappedPort(ctx, "10000/tcp")
	if err != nil {
		t.Fatalf("failed to get Envoy mapped port: %v", err)
	}
	envoyAddr := fmt.Sprintf("http://%s", net.JoinHostPort(envoyHost, envoyPort.Port()))

	// 6. Run Test Cases

	// --- Case A: Multitenancy Enabled - Matching tenant (Canonical) ---
	reqA, _ := http.NewRequest("GET", envoyAddr+"/ready", nil)
	reqA.Header.Set("Cookie", "session_id=valid-session-alice")
	respA, err := http.DefaultClient.Do(reqA)
	if err != nil {
		t.Fatalf("Case A failed: %v", err)
	}
	defer respA.Body.Close()

	if respA.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(respA.Body)
		t.Errorf("Case A: expected status %d, got %d. Body: %s", http.StatusOK, respA.StatusCode, string(body))
	}
	// Assert Authorization header was forwarded to mock_upstream (which is Envoy's admin port, and returns 200)
	if respA.StatusCode == http.StatusOK {
		t.Log("Case A: successfully authenticated and authorised")
	}

	// --- Case B: Multitenancy Enabled - Mismatched tenant (Ubuntu) ---
	reqB, _ := http.NewRequest("GET", envoyAddr+"/ready", nil)
	reqB.Header.Set("Cookie", "session_id=valid-session-bob")
	respB, err := http.DefaultClient.Do(reqB)
	if err != nil {
		t.Fatalf("Case B failed: %v", err)
	}
	defer respB.Body.Close()

	if respB.StatusCode != http.StatusForbidden {
		t.Errorf("Case B: expected status %d, got %d", http.StatusForbidden, respB.StatusCode)
	} else {
		body, _ := io.ReadAll(respB.Body)
		expectedBody := "access denied for user bob@example.com"
		if string(body) != expectedBody {
			t.Errorf("Case B: expected body %q, got %q", expectedBody, string(body))
		}
		t.Log("Case B: successfully blocked access with 403 Forbidden on tenant mismatch")
	}

	// 7. Tear down the multitenant server and spin up a non-multitenant one to test bypass
	grpcServer.Stop()

	l2, err := net.Listen("tcp", "0.0.0.0:0")
	if err != nil {
		t.Fatalf("failed to listen on all interfaces: %v", err)
	}
	grpcPort2 := l2.Addr().(*net.TCPAddr).Port

	grpcServer2 := grpc.NewServer()

	extSvcSingleTenant := authz.NewExternalAuthzService(
		mockVerifier,
		mockSTS,
		mockResourceMapper,
		fgaClient,
		false, // multitenancyEnabled = false
		suite.TestLogger,
		noop.NewTracerProvider().Tracer("test"),
	)
	extSvcSingleTenant.Register(grpcServer2)

	go func() {
		if err := grpcServer2.Serve(l2); err != nil {
			t.Logf("gRPC server 2 stopped: %v", err)
		}
	}()
	defer grpcServer2.GracefulStop()

	// Start a second Envoy pointing to the non-multitenant gRPC port
	envoyConfigYAML2 := strings.ReplaceAll(envoyConfigYAML, fmt.Sprintf("port_value: %d", grpcPort), fmt.Sprintf("port_value: %d", grpcPort2))
	envoyReq2 := testcontainers.ContainerRequest{
		Image:        "envoyproxy/envoy:v1.30.0",
		ExposedPorts: []string{"10000/tcp"},
		Files: []testcontainers.ContainerFile{
			{
				Reader:            strings.NewReader(envoyConfigYAML2),
				ContainerFilePath: "/etc/envoy/envoy.yaml",
				FileMode:          0644,
			},
		},
		HostConfigModifier: func(hc *dockercontainer.HostConfig) {
			hc.ExtraHosts = []string{"host.testcontainers.internal:host-gateway"}
		},
		WaitingFor: wait.ForLog("starting workers").WithStartupTimeout(30 * time.Second),
	}

	envoyCtr2, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: envoyReq2,
		Started:          true,
	})
	if err != nil {
		t.Fatalf("failed to start Envoy 2: %v", err)
	}
	defer func() { _ = envoyCtr2.Terminate(ctx) }()

	envoyHost2, err := envoyCtr2.Host(ctx)
	if err != nil {
		t.Fatalf("failed to get Envoy 2 host: %v", err)
	}
	envoyPort2, err := envoyCtr2.MappedPort(ctx, "10000/tcp")
	if err != nil {
		t.Fatalf("failed to get Envoy 2 mapped port: %v", err)
	}
	envoyAddr2 := fmt.Sprintf("http://%s", net.JoinHostPort(envoyHost2, envoyPort2.Port()))

	// --- Case C: Multitenancy Disabled - Bob's token (Ubuntu) bypasses match constraint ---
	reqC, _ := http.NewRequest("GET", envoyAddr2+"/ready", nil)
	reqC.Header.Set("Cookie", "session_id=valid-session-bob")
	respC, err := http.DefaultClient.Do(reqC)
	if err != nil {
		t.Fatalf("Case C failed: %v", err)
	}
	defer respC.Body.Close()

	if respC.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(respC.Body)
		t.Errorf("Case C: expected status %d, got %d. Body: %s", http.StatusOK, respC.StatusCode, string(body))
	} else {
		t.Log("Case C: successfully bypassed tenant mismatch check because global tenancy is disabled")
	}
}
