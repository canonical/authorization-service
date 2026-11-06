# Tasks

## 1. Protobuf & gRPC Stub Generation (`api/proto/v1`, `api/v1`)

- [x] 1.1 Define `PermissionIngestionService` with `rpc IngestPermissionUpdate(PermissionUpdateEnvelope) returns (IngestPermissionUpdateResponse)` and `message IngestPermissionUpdateResponse {}` in `api/proto/v1/messages.proto` (updating `buf.yaml` if needed for lint rules), and generate Go protobuf and gRPC stubs in `api/v1/messages.pb.go` and `api/v1/messages_grpc.pb.go` via `make protogen` (verified by `go build ./api/v1/...`).

## 2. Mode-Aware Configuration & Validation (`config`)

- [x] 2.1 Introduce `ListenerConfig` (`listener:` YAML tree with `mode`, `federated_services_strategy`, and `federated_services`) and `ComponentListener` in `config/specs.go` (decoupling `Listener` from `KafkaConfig`), bind `listener.*` keys and env/flag aliases in `config/viper.go`, and update `BuildServiceRegistry(cfg *ListenerConfig)`, `InitServiceRegistry`, `InitKafkaConsumer`, and `NewOIDCVerifier` in `config/helper.go`; add unit tests in `config/specs_test.go`, `config/viper_test.go`, and `config/helper_test.go`.

## 3. Envelope Ingestion & Authenticated gRPC Handler (`internal/service/listen`)

- [x] 3.1 Add `HasService` to `internal/service/listen/registry.go` and `IngestEnvelope(ctx context.Context, env *messagesv1.PermissionUpdateEnvelope) error` to `internal/service/listen/ingestor.go` so decoded gRPC envelopes are validated against the federated service registry, marshaled, and persisted to `permission_update_work` with idempotency and metrics; add unit tests in `internal/service/listen/ingestor_test.go` and `internal/service/listen/registry_test.go`.
- [x] 3.2 Implement `GRPCIngestionServer` in `internal/service/listen/grpc.go` performing Bearer JWT extraction from gRPC metadata, signature verification via `authz.OidcVerifier` (`codes.Unauthenticated` on missing/invalid token), and delegation to `IngestEnvelope` (`codes.InvalidArgument` on permanent errors, `codes.Internal` on transient errors, `codes.OK` on success/duplicate); add unit tests in `internal/service/listen/grpc_test.go`.

## 4. gRPC Server Registration & CLI Supervision (`server/grpc`, `cmd`)

- [x] 4.1 Add `WithPermissionIngestionService` option to `server/grpc/server.go` to register `PermissionIngestionServiceServer` on the gRPC server, and add unit test coverage in `server/grpc/middleware_test.go` (or `server/grpc/server_test.go`).
- [x] 4.2 Update `cmd/listen.go`, `cmd/list_topics.go`, and `cmd/ensure_topics.go` to use `config.ComponentListener` and `cfg.Listener`, register `--mode` on `listen`, and conditionally start/supervise the Kafka consumer pipeline (`platform`, `hybrid`) and the gRPC ingestion server (`standalone`, `hybrid`) alongside the health and metrics servers; update unit tests in `cmd/listen_test.go` and `cmd/list_topics_test.go`.

## 5. Integration Testing & Verification (`tests/integration/listener`)

- [x] 5.1 Add testcontainers-based integration tests in `tests/integration/listener/listener_test.go` covering `standalone` gRPC ingestion with a local JWKS server (valid Bearer JWT, duplicate delivery, invalid service slug, missing/invalid/expired JWT returning `codes.Unauthenticated`) and `hybrid` concurrent Kafka + gRPC ingestion.
- [x] 5.2 Run the verification suite (`go test -race ./...` and `go test -v -race -tags integration ./tests/integration/listener/...`) ensuring zero diagnostic failures.

## 6. Documentation & Rollout

- [x] 6.1 Update `authorization-service.yaml.example`, `.env.example`, `LOCAL_DEVELOPMENT.md`, and `README.md` with the new `listener` configuration section, and verify `openspec validate "support-listener-ingestion-modes" --strict`.

## Implementation Notes

- Moved `mode`, `federated_services_strategy`, and `federated_services` into a dedicated `ListenerConfig` (`listener:` YAML section) so `Listener` does not depend on `KafkaConfig`.
