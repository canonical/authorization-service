# Proposal

## Why

Currently, the `listen` command operates exclusively by consuming permission-update envelopes from federated Kafka topics, and federated service registry properties live inside `KafkaConfig`. To support deployments where federated services push permission updates directly over gRPC without relying on Kafka—or alongside Kafka—the listener needs its own dedicated `listener` configuration section (`mode`, `federated_services_strategy`, `federated_services`), configurable operating modes (`platform`, `standalone`, and `hybrid`) with a JWT-authenticated gRPC ingestion endpoint, and mode-aware configuration validation decoupled from `KafkaConfig`.

## What Changes

- Introduce a dedicated `ListenerConfig` (`listener:` YAML tree, `ComponentListener`) in `config` holding `mode` (`platform`, `standalone`, `hybrid`, default `platform`), `federated_services_strategy` (`auto`, `fs`, `config`, default `auto`), and `federated_services`, removing federated service registry fields from `KafkaConfig`.
- Rework `listen` configuration validation via `ComponentListener` in `config`:
  - Validates `ListenerConfig` (`mode` and federated service registry strategy/slugs) without touching `KafkaConfig`.
  - `platform`: implicitly requires and validates `KafkaConfig` (brokers and consumer group); does not require JWKS configuration.
  - `standalone`: requires JWKS configuration (`EXTAUTHZ_JWK_SET_URL`); does not require or inspect `KafkaConfig`.
  - `hybrid`: requires both `KafkaConfig` and JWKS configurations.
- Define `PermissionIngestionService.IngestPermissionUpdate` in `api/proto/v1/messages.proto` accepting `PermissionUpdateEnvelope` directly, and generate Go gRPC stubs in `api/v1`.
- Extend `internal/service/listen` and `server/grpc` to expose the gRPC ingestion server, authenticating Bearer JWTs against the configured Ory Hydra JWKS endpoint and persisting valid envelopes to `permission_update_work` via `internal/repository`.
- Update `cmd/listen.go`, `cmd/list_topics.go`, and `cmd/ensure_topics.go` to use `ListenerConfig` and `ComponentListener`.

## Non-goals

- Synchronous application of permission updates to OpenFGA during gRPC ingestion (updates remain asynchronously applied by `worker`).
- Exposing the permission ingestion endpoint on the `serve` command or via the REST gateway.
- Replacing or modifying Envoy `ext_authz` STS token exchange in `internal/service/authz`.

## Capabilities

### New Capabilities
- `permission-update-ingestion`: Configurable listener ingestion modes (`platform`, `standalone`, `hybrid`), gRPC permission-update ingestion endpoint, and Ory Hydra JWKS Bearer token authentication.

### Modified Capabilities
- `command-config-validation`: Dedicated `listener` configuration tree and mode-aware validation for the `listen` and `topics` commands across `platform`, `standalone`, and `hybrid` modes.

## Impact

- **Affected packages**: `api/proto/v1`, `api/v1`, `config`, `cmd`, `internal/service/listen`, `server/grpc`, `tests/integration/listener`.
- **Backward compatibility**: Defaults to `platform` mode and supports legacy `FEDERATED_SERVICES*` / `KAFKA_FEDERATED_SERVICES*` environment variable aliases.

## Success Metrics

- 100% of unit and integration tests pass (`go test -race ./...` and `./tests/integration/listener/...`) across `platform`, `standalone`, and `hybrid` modes.
- Unauthenticated or malformed gRPC ingestion requests are rejected with `Unauthenticated` or `InvalidArgument` before any database write occurs.
