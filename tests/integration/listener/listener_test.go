//go:build integration

package listener

import (
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net"
	"math/big"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/prometheus/client_golang/prometheus"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"

	messagesv1 "github.com/canonical/authorization-service/api/v1"
	"github.com/canonical/authorization-service/config"
	"github.com/canonical/authorization-service/internal/health"
	kafkaintegration "github.com/canonical/authorization-service/internal/integration/kafka"
	"github.com/canonical/authorization-service/internal/integration/postgres"
	"github.com/canonical/authorization-service/internal/metrics"
	"github.com/canonical/authorization-service/internal/repository"
	"github.com/canonical/authorization-service/internal/service/listen"
	"github.com/canonical/authorization-service/tests/integration/suite"
	grpcserver "github.com/canonical/authorization-service/server/grpc"
)

// newTestPostgres opens a postgres.Client and a raw pool against the test DB.
func newTestPostgres(t *testing.T) (*postgres.Client, *pgxpool.Pool) {
	return suite.NewTestPostgres(t, pgDSN, pgConfig)
}

// newTestListener wires a Listener backed by the test containers, subscribing to
// all federated services within a unique consumer group.
func newTestListener(t *testing.T, group string, db postgres.DBClientInterface) *listen.Listener {
	t.Helper()

	registry, err := listen.NewServiceRegistry(suite.FederatedServices)
	if err != nil {
		t.Fatalf("registry: %v", err)
	}

	kafkaClient, err := kafkaintegration.NewClient(kafkaintegration.Config{
		Brokers:       []string{kafkaBroker},
		ConsumerGroup: group,
		Topics:        registry.Topics(),
	}, suite.TestLogger)
	if err != nil {
		t.Fatalf("newTestListener kafka client: %v", err)
	}
	t.Cleanup(func() { kafkaClient.Close() })

	repo := repository.NewPostgresPermissionWorkRepository(db, nil)
	ingestor := listen.NewIngestionService(
		registry, listen.NewDecoder(), listen.NewValidator(), repo, nil, suite.TestLogger,
	)
	return listen.NewListener(kafkaClient, ingestor, suite.TestLogger)
}

func startListener(ctx context.Context, l *listen.Listener) <-chan error {
	ch := make(chan error, 1)
	go func() { ch <- l.Run(ctx) }()
	return ch
}

// countWorkRows returns how many permission_update_work rows match.
func countWorkRows(t *testing.T, pool *pgxpool.Pool, service, idempotencyKey string) int {
	t.Helper()
	var n int
	err := pool.QueryRow(context.Background(),
		`SELECT count(*) FROM permission_update_work WHERE service = $1 AND idempotency_key = $2`,
		service, idempotencyKey,
	).Scan(&n)
	if err != nil {
		t.Fatalf("countWorkRows: %v", err)
	}
	return n
}

// waitForWorkRow polls until a matching row appears in status 'received'.
func waitForWorkRow(t *testing.T, pool *pgxpool.Pool, service, idempotencyKey string) {
	t.Helper()
	deadline := time.Now().Add(20 * time.Second)
	for time.Now().Before(deadline) {
		var status string
		err := pool.QueryRow(context.Background(),
			`SELECT status FROM permission_update_work WHERE service = $1 AND idempotency_key = $2`,
			service, idempotencyKey,
		).Scan(&status)
		if err == nil {
			if status != "received" {
				t.Fatalf("row status = %q, want received", status)
			}
			return
		}
		time.Sleep(300 * time.Millisecond)
	}
	t.Fatalf("work row (service=%s idempotency_key=%s) not found within 20s", service, idempotencyKey)
}

// TestListener_HappyPath publishes one valid envelope and asserts a 'received'
// row is durably persisted.
func TestListener_HappyPath(t *testing.T) {
	suffix := suite.UniqueSuffix()
	idem := "idem-happy-" + suffix

	client, pool := newTestPostgres(t)
	suite.PublishEnvelope(t, kafkaBroker, "payments", suite.SampleEnvelope("payments", idem))

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	errCh := startListener(ctx, newTestListener(t, "happy-"+suffix, client))

	waitForWorkRow(t, pool, "payments", idem)

	cancel()
	if err := <-errCh; err != nil {
		t.Fatalf("listener shutdown error: %v", err)
	}
}

// TestListener_DuplicateDelivery publishes the same idempotency key twice and
// asserts only a single row exists.
func TestListener_DuplicateDelivery(t *testing.T) {
	suffix := suite.UniqueSuffix()
	idem := "idem-dup-" + suffix

	client, pool := newTestPostgres(t)
	suite.PublishEnvelope(t, kafkaBroker, "payments", suite.SampleEnvelope("payments", idem))
	suite.PublishEnvelope(t, kafkaBroker, "payments", suite.SampleEnvelope("payments", idem))

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	errCh := startListener(ctx, newTestListener(t, "dup-"+suffix, client))

	waitForWorkRow(t, pool, "payments", idem)
	// Give the second delivery time to be processed as an idempotent no-op.
	time.Sleep(2 * time.Second)
	if n := countWorkRows(t, pool, "payments", idem); n != 1 {
		t.Fatalf("expected exactly 1 row, got %d", n)
	}

	cancel()
	<-errCh
}

// TestListener_ServiceMismatchNotPersisted publishes an envelope whose service
// field does not match the topic slug; it must fail validation and not persist.
func TestListener_ServiceMismatchNotPersisted(t *testing.T) {
	suffix := suite.UniqueSuffix()
	idem := "idem-mismatch-" + suffix

	client, pool := newTestPostgres(t)

	env := suite.SampleEnvelope("invoicing", idem) // service=invoicing...
	suite.PublishEnvelope(t, kafkaBroker, "payments", env)      // ...published to permissions.payments

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	errCh := startListener(ctx, newTestListener(t, "mismatch-"+suffix, client))

	// A permanent validation failure is acknowledged, not persisted.
	time.Sleep(3 * time.Second)
	if n := countWorkRows(t, pool, "invoicing", idem); n != 0 {
		t.Fatalf("expected 0 rows for mismatched service, got %d", n)
	}

	cancel()
	<-errCh
}

// TestListener_HealthzEndpoint verifies that the health check HTTP server serves
// GET /healthz with 200 OK (`{"status":"SERVING"}`) while the listener is actively
// consuming from Kafka and persisting to Postgres, both with and without the
// metrics server running alongside it, and stops accepting connections after
// graceful shutdown.
func TestListener_HealthzEndpoint(t *testing.T) {
	for _, tc := range []struct {
		name           string
		metricsEnabled bool
	}{
		{name: "with_metrics_enabled", metricsEnabled: true},
		{name: "with_metrics_disabled", metricsEnabled: false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			suffix := suite.UniqueSuffix()
			idem := "idem-healthz-" + tc.name + "-" + suffix

			client, pool := newTestPostgres(t)
			suite.PublishEnvelope(t, kafkaBroker, "payments", suite.SampleEnvelope("payments", idem))

			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			listenerErrCh := startListener(ctx, newTestListener(t, "healthz-"+tc.name+"-"+suffix, client))

			healthLn, err := net.Listen("tcp", "127.0.0.1:0")
			if err != nil {
				t.Fatalf("listen health: %v", err)
			}
			healthSrv := health.NewServer(healthLn.Addr().String())
			healthErrCh := make(chan error, 1)
			go func() {
				healthErrCh <- healthSrv.Serve(healthLn)
			}()

			var metricsSrv *http.Server
			var metricsErrCh chan error
			var metricsURL string
			if tc.metricsEnabled {
				metricsLn, err := net.Listen("tcp", "127.0.0.1:0")
				if err != nil {
					t.Fatalf("listen metrics: %v", err)
				}
				metricsURL = "http://" + metricsLn.Addr().String() + "/metrics"
				reg := prometheus.NewRegistry()
				metricsSrv = metrics.NewServer(metricsLn.Addr().String(), "/metrics", reg)
				metricsErrCh = make(chan error, 1)
				go func() {
					metricsErrCh <- metricsSrv.Serve(metricsLn)
				}()
			}

			waitForWorkRow(t, pool, "payments", idem)

			healthURL := "http://" + healthLn.Addr().String() + "/healthz"
			resp, err := http.Get(healthURL)
			if err != nil {
				t.Fatalf("GET /healthz failed: %v", err)
			}
			body, err := io.ReadAll(resp.Body)
			_ = resp.Body.Close()
			if err != nil {
				t.Fatalf("read /healthz body: %v", err)
			}
			if resp.StatusCode != http.StatusOK {
				t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusOK)
			}
			if got := resp.Header.Get("Content-Type"); got != "application/json" {
				t.Fatalf("Content-Type = %q, want %q", got, "application/json")
			}
			if string(body) != `{"status":"SERVING"}` {
				t.Fatalf("body = %q, want %q", string(body), `{"status":"SERVING"}`)
			}

			if tc.metricsEnabled {
				mResp, err := http.Get(metricsURL)
				if err != nil {
					t.Fatalf("GET /metrics failed: %v", err)
				}
				_ = mResp.Body.Close()
				if mResp.StatusCode != http.StatusOK {
					t.Fatalf("metrics status = %d, want %d", mResp.StatusCode, http.StatusOK)
				}
			}

			shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer shutdownCancel()
			if err := healthSrv.Shutdown(shutdownCtx); err != nil {
				t.Fatalf("healthSrv.Shutdown: %v", err)
			}
			if err := <-healthErrCh; err != nil && err != http.ErrServerClosed {
				t.Fatalf("healthSrv.Serve returned unexpected error: %v", err)
			}

			if metricsSrv != nil {
				if err := metricsSrv.Shutdown(shutdownCtx); err != nil {
					t.Fatalf("metricsSrv.Shutdown: %v", err)
				}
				if err := <-metricsErrCh; err != nil && err != http.ErrServerClosed {
					t.Fatalf("metricsSrv.Serve returned unexpected error: %v", err)
				}
			}

			cancel()
			if err := <-listenerErrCh; err != nil {
				t.Fatalf("listener shutdown error: %v", err)
			}

			// Verify /healthz refuses connections after shutdown.
			if resp, err := http.Get(healthURL); err == nil {
				_ = resp.Body.Close()
				t.Fatalf("expected GET /healthz to fail after shutdown, got status %d", resp.StatusCode)
			}
		})
	}
}

// startTestJWKSServer generates an RSA keypair and starts an httptest.Server
// serving a JWKS containing the public key under kid "test-hydra-key".
func startTestJWKSServer(t *testing.T) (*rsa.PrivateKey, *httptest.Server) {
	t.Helper()
	privKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate RSA key: %v", err)
	}

	jwksPayload, err := json.Marshal(map[string]any{
		"keys": []map[string]any{
			{
				"kty": "RSA",
				"alg": "RS256",
				"use": "sig",
				"kid": "test-hydra-key",
				"n":   base64.RawURLEncoding.EncodeToString(privKey.N.Bytes()),
				"e":   base64.RawURLEncoding.EncodeToString(big.NewInt(int64(privKey.E)).Bytes()),
			},
		},
	})
	if err != nil {
		t.Fatalf("marshal JWKS: %v", err)
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(jwksPayload)
	}))
	t.Cleanup(srv.Close)
	return privKey, srv
}

// signTestRS256JWT signs a minimal JWT with the given RSA private key and expiration.
func signTestRS256JWT(t *testing.T, privKey *rsa.PrivateKey, exp time.Time) string {
	t.Helper()
	headerJSON, err := json.Marshal(map[string]any{
		"alg": "RS256",
		"typ": "JWT",
		"kid": "test-hydra-key",
	})
	if err != nil {
		t.Fatalf("marshal JWT header: %v", err)
	}
	claimsJSON, err := json.Marshal(map[string]any{
		"sub": "payments-service",
		"iat": time.Now().Add(-1 * time.Minute).Unix(),
		"exp": exp.Unix(),
	})
	if err != nil {
		t.Fatalf("marshal JWT claims: %v", err)
	}

	signingInput := base64.RawURLEncoding.EncodeToString(headerJSON) + "." + base64.RawURLEncoding.EncodeToString(claimsJSON)
	digest := sha256.Sum256([]byte(signingInput))
	sig, err := rsa.SignPKCS1v15(rand.Reader, privKey, crypto.SHA256, digest[:])
	if err != nil {
		t.Fatalf("sign JWT: %v", err)
	}
	return signingInput + "." + base64.RawURLEncoding.EncodeToString(sig)
}

func startTestGRPCIngestionServer(t *testing.T, ctx context.Context, jwksURL string, db postgres.DBClientInterface) (messagesv1.PermissionIngestionServiceClient, *grpcserver.Server, <-chan error) {
	t.Helper()

	registry, err := listen.NewServiceRegistry(suite.FederatedServices)
	if err != nil {
		t.Fatalf("registry: %v", err)
	}
	repo := repository.NewPostgresPermissionWorkRepository(db, nil)
	ingestor := listen.NewIngestionService(
		registry, listen.NewDecoder(), listen.NewValidator(), repo, nil, suite.TestLogger,
	)
	verifier := config.NewOIDCVerifier(ctx, jwksURL)
	grpcIngest := listen.NewGRPCIngestionServer(verifier, ingestor, suite.TestLogger)

	srv, err := grpcserver.NewServer(
		&config.ServerConfig{Host: "127.0.0.1", GRPCPort: 0},
		suite.TestLogger,
		metrics.NewRegistry(),
		grpcserver.WithPermissionIngestionService(grpcIngest),
	)
	if err != nil {
		t.Fatalf("grpcserver.NewServer: %v", err)
	}

	errCh := make(chan error, 1)
	go func() {
		errCh <- srv.Start()
	}()
	t.Cleanup(srv.GracefulStop)

	conn, err := grpc.NewClient(srv.Addr(), grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatalf("grpc.NewClient: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })

	return messagesv1.NewPermissionIngestionServiceClient(conn), srv, errCh
}

func grpcBearerCtx(ctx context.Context, token string) context.Context {
	return metadata.AppendToOutgoingContext(ctx, "authorization", "Bearer "+token)
}

// TestListener_StandaloneGRPCMode exercises the standalone gRPC ingestion server
// backed by PostgreSQL and an HTTP JWKS endpoint without any Kafka consumer.
func TestListener_StandaloneGRPCMode(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	privKey, jwksSrv := startTestJWKSServer(t)
	pgClient, pool := newTestPostgres(t)
	grpcClient, srv, errCh := startTestGRPCIngestionServer(t, ctx, jwksSrv.URL, pgClient)

	validToken := signTestRS256JWT(t, privKey, time.Now().Add(10*time.Minute))

	t.Run("valid_bearer_token_persists_envelope", func(t *testing.T) {
		idem := "idem-grpc-happy-" + suite.UniqueSuffix()
		env := suite.SampleEnvelope("payments", idem)

		_, err := grpcClient.IngestPermissionUpdate(grpcBearerCtx(ctx, validToken), env)
		if err != nil {
			t.Fatalf("IngestPermissionUpdate failed: %v", err)
		}

		waitForWorkRow(t, pool, "payments", idem)
	})

	t.Run("duplicate_delivery_is_idempotent", func(t *testing.T) {
		idem := "idem-grpc-dup-" + suite.UniqueSuffix()
		env := suite.SampleEnvelope("payments", idem)

		if _, err := grpcClient.IngestPermissionUpdate(grpcBearerCtx(ctx, validToken), env); err != nil {
			t.Fatalf("first IngestPermissionUpdate failed: %v", err)
		}
		if _, err := grpcClient.IngestPermissionUpdate(grpcBearerCtx(ctx, validToken), env); err != nil {
			t.Fatalf("duplicate IngestPermissionUpdate failed: %v", err)
		}

		if n := countWorkRows(t, pool, "payments", idem); n != 1 {
			t.Fatalf("expected 1 row for duplicate gRPC delivery, got %d", n)
		}
	})

	t.Run("unknown_service_rejected_with_invalid_argument", func(t *testing.T) {
		idem := "idem-grpc-unknown-" + suite.UniqueSuffix()
		env := suite.SampleEnvelope("unregistered", idem)

		_, err := grpcClient.IngestPermissionUpdate(grpcBearerCtx(ctx, validToken), env)
		if status.Code(err) != codes.InvalidArgument {
			t.Fatalf("status code = %v, want %v (err=%v)", status.Code(err), codes.InvalidArgument, err)
		}
		if n := countWorkRows(t, pool, "unregistered", idem); n != 0 {
			t.Fatalf("expected 0 rows for unknown service, got %d", n)
		}
	})

	t.Run("unauthenticated_requests_rejected", func(t *testing.T) {
		idem := "idem-grpc-unauth-" + suite.UniqueSuffix()
		env := suite.SampleEnvelope("payments", idem)

		// 1. Missing authorization metadata
		if _, err := grpcClient.IngestPermissionUpdate(ctx, env); status.Code(err) != codes.Unauthenticated {
			t.Fatalf("missing metadata: got %v, want Unauthenticated", status.Code(err))
		}

		// 2. Signed by wrong RSA key
		otherKey, err := rsa.GenerateKey(rand.Reader, 2048)
		if err != nil {
			t.Fatalf("generate other RSA key: %v", err)
		}
		wrongKeyToken := signTestRS256JWT(t, otherKey, time.Now().Add(10*time.Minute))
		if _, err := grpcClient.IngestPermissionUpdate(grpcBearerCtx(ctx, wrongKeyToken), env); status.Code(err) != codes.Unauthenticated {
			t.Fatalf("wrong key token: got %v, want Unauthenticated", status.Code(err))
		}

		// 3. Expired token
		expiredToken := signTestRS256JWT(t, privKey, time.Now().Add(-5*time.Minute))
		if _, err := grpcClient.IngestPermissionUpdate(grpcBearerCtx(ctx, expiredToken), env); status.Code(err) != codes.Unauthenticated {
			t.Fatalf("expired token: got %v, want Unauthenticated", status.Code(err))
		}

		if n := countWorkRows(t, pool, "payments", idem); n != 0 {
			t.Fatalf("expected 0 rows for unauthenticated requests, got %d", n)
		}
	})

	srv.GracefulStop()
	if err := <-errCh; err != nil && !errors.Is(err, net.ErrClosed) {
		t.Fatalf("gRPC server shutdown error: %v", err)
	}
}

// TestListener_HybridMode runs both the Kafka listener and the gRPC ingestion
// server concurrently, verifying that updates arriving on both transports are
// durably persisted into PostgreSQL.
func TestListener_HybridMode(t *testing.T) {
	suffix := suite.UniqueSuffix()
	idemKafka := "idem-hybrid-kafka-" + suffix
	idemGRPC := "idem-hybrid-grpc-" + suffix

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	privKey, jwksSrv := startTestJWKSServer(t)
	pgClient, pool := newTestPostgres(t)

	listenerErrCh := startListener(ctx, newTestListener(t, "hybrid-"+suffix, pgClient))
	grpcClient, grpcSrv, grpcErrCh := startTestGRPCIngestionServer(t, ctx, jwksSrv.URL, pgClient)

	suite.PublishEnvelope(t, kafkaBroker, "payments", suite.SampleEnvelope("payments", idemKafka))

	validToken := signTestRS256JWT(t, privKey, time.Now().Add(10*time.Minute))
	if _, err := grpcClient.IngestPermissionUpdate(grpcBearerCtx(ctx, validToken), suite.SampleEnvelope("invoicing", idemGRPC)); err != nil {
		t.Fatalf("gRPC IngestPermissionUpdate in hybrid mode failed: %v", err)
	}

	waitForWorkRow(t, pool, "payments", idemKafka)
	waitForWorkRow(t, pool, "invoicing", idemGRPC)

	cancel()
	grpcSrv.GracefulStop()
	if err := <-listenerErrCh; err != nil {
		t.Fatalf("kafka listener shutdown error: %v", err)
	}
	if err := <-grpcErrCh; err != nil && !errors.Is(err, net.ErrClosed) {
		t.Fatalf("gRPC server shutdown error: %v", err)
	}
}

