# Tasks

## 1. Protobuf, gRPC, `grpc-gateway`, and OpenAPI Generation (`api/proto/v1`, `api/v1`, `openapi`)

- [ ] 1.1 Define `TupleService` (`WriteTuples`, `DeleteTuples`, `ListTuples`, `Check`, `GetAuthorizationModel` using `google.api.HttpBody`) with `google.api.http` annotations (`POST /api/v1/tuples`, `DELETE /api/v1/tuples`, `GET /api/v1/tuples`, `POST /api/v1/check`, `GET /api/v1/model`) in `api/proto/v1/tuples.proto` without modifying existing `.proto` files; run `make protogen` and `make openapi-v3` to generate `api/v1/tuples*.go` and update `openapi/api.swagger.json` and `openapi/api.yaml` (verified by `go build ./api/v1/...` and `buf lint`).

## 2. Database Migration (`migrations`)

- [ ] 2.1 Add `migrations/00004_add_authorization_tuples_conditions_and_indexes.sql` adding nullable `condition_name TEXT`, `condition_context JSONB`, `CHECK (condition_context IS NULL OR condition_name IS NOT NULL)`, `idx_authorization_tuples_service_id`, and `idx_authorization_tuples_service_object` on `authorization_tuples` (keeping `service` as `TEXT NOT NULL` with no FK); verify up/down migration cleanly in integration tests.

## 3. Shared Tuple Model & Validation (`internal/model/permissions`, `internal/service/listen`)

- [ ] 3.1 Add `ConditionName *string` and `ConditionContext map[string]any` to `permissions.Tuple`, add `StoredTuple`, `TupleListFilter`, `TupleListPage`, and shared tuple syntax validation helpers (`ValidateTuple`, `ValidateSubject`, `ValidateTypeID`) in `internal/model/permissions/work.go` and `internal/model/permissions/validate.go`, with unit tests in `internal/model/permissions/validate_test.go`.
- [ ] 3.2 Update `internal/service/listen/validator.go` to delegate tuple syntax checks to `internal/model/permissions`, verifying existing unit tests in `internal/service/listen/validator_test.go` pass.

## 4. Mode-Aware Worker Configuration & Dual-JWKS Validation (`config`)

- [ ] 4.1 Extend `WorkerConfig` in `config/specs.go` with `Mode` (`platform`, `standalone`, `hybrid`, default `platform`), `ServiceAccountJwkSetURL`, `AuthType` (`OIDC`), `ServiceIdentity`, `ServiceIdentities`, and `GetMode()`; update `componentValidators[ComponentWorker]` to require both `ext_authz_service.jwk_set_url` and `worker.service_account_jwk_set_url` and validate `ServiceIdentities` in `standalone` and `hybrid` modes; bind `worker.mode` (`WORKER_MODE`, `--mode`), `worker.service_account_jwk_set_url` (`WORKER_SERVICE_ACCOUNT_JWK_SET_URL`), and JSON/YAML `worker.service_identities` (`WORKER_SERVICE_IDENTITIES`) in `config/viper.go`; populate `jwkSetUrl` (and `serviceAccountJwkSetUrl`) for `ComponentWorker` in `standalone`/`hybrid` modes in `config/helper.go`; add unit tests in `config/specs_test.go`, `config/viper_test.go`, and `config/helper_test.go`.

## 5. Tuple Repository & Async Condition Parity (`internal/repository`, `internal/service/worker`)

- [ ] 5.1 Update `PostgresPermissionWorkRepository.RecordProcessed` in `internal/repository/permission_work.go` and `toModelTuples` in `internal/service/worker/processor.go` so async writes persist `condition_name` and `condition_context` into `authorization_tuples` when present; update unit tests in `internal/repository/permission_work_test.go` and `internal/service/worker/processor_test.go`.
- [ ] 5.2 Define `SyncTupleRepository` and implement `ApplyWrites` (persisting `condition_name` and `condition_context`), `ApplyDeletes` (staging in PG tx, invoking OpenFGA callback before commit, rolling back on callback error, returning typed post-OpenFGA commit error if `tx.Commit` fails), and `ListByService` (filtering strictly by `service` + optional `subject`/`relation`/`object`, ordered by `id ASC`, returning `condition_name`/`condition_context` and `total_count`) in `internal/repository/tuples.go` with `pgxmock` unit tests in `internal/repository/tuples_test.go`.

## 6. Observability Metrics (`internal/metrics`)

- [ ] 6.1 Add Prometheus metrics recorder for synchronous worker API operations (`write`, `delete`, `list`, `check`, `model`) and outcomes in `internal/metrics/worker.go` (or `internal/metrics/tuples.go`) with unit tests in `internal/metrics/worker_test.go`.

## 7. Principal Auth, Sync Service, Model Cache, and gRPC Handler (`internal/service/worker`)

- [ ] 7.1 Implement `Principal` (`UserAccount` vs `ServiceAccount`), `DualJWKSAuthenticator` (verifying against `serviceAccountVerifier` -> `ServiceAccount` and `userAccountVerifier` -> `UserAccount`, extracting `client_id` fallback `sub` and `org`), and `StaticServiceIdentityResolver` in `internal/service/worker/principal.go` and `internal/service/worker/identity.go`, with unit tests in `internal/service/worker/principal_test.go` and `internal/service/worker/identity_test.go`.
- [ ] 7.2 Implement `SyncTupleService` (enforcing batch limit 50, tuple validation, `jwt.org` `tenant_match` condition when `multitenancyEnabled=true`, `OpenFGA.Timeout`, `TupleApplier`, `BatchCheck` proxy, and `SyncTupleRepository`) and `CachedModelReader` (`ReadAuthorizationModel` cached in memory with JSON and `transformer.TransformJSONStringToDSL` indented DSL support) in `internal/service/worker/sync_tuples.go` and `internal/service/worker/model_cache.go`, with unit tests and generated mocks (`//go:generate mockgen`) in `internal/service/worker/sync_tuples_test.go` and `internal/service/worker/model_cache_test.go`.
- [ ] 7.3 Implement `TupleGRPCServer` (`api/v1.TupleServiceServer`) in `internal/service/worker/grpc.go` enforcing endpoint auth boundaries (`GetAuthorizationModel` public; `Check` accepts `UserAccount` + `ServiceAccount`; `WriteTuples`/`DeleteTuples`/`ListTuples` require `ServiceAccount` mapped in `ServiceIdentityResolver`) and mapping domain errors to gRPC status codes (`Unauthenticated`, `PermissionDenied`, `InvalidArgument`, `Unavailable`, `Internal`), with unit tests in `internal/service/worker/grpc_test.go`.

## 8. Server Registration & Worker CLI Wiring (`server/grpc`, `server/rest`, `cmd`)

- [ ] 8.1 Add `WithTupleService` option to `server/grpc/server.go` and `WithHandlerRegistrar` option to `server/rest/gateway.go` (preserving `GET /healthz` and existing `cmd/serve.go` behavior), with unit tests in `server/grpc/middleware_test.go` and `server/rest/gateway_test.go`.
- [ ] 8.2 Update `cmd/worker.go` to register `--mode`, keep `platform` mode unchanged (async `Worker` + `Reaper` + `health.NewServer`), and in `standalone`/`hybrid` modes wire `DualJWKSAuthenticator`, `StaticServiceIdentityResolver`, `PostgresTupleRepository`, `SyncTupleService`, `CachedModelReader`, `TupleGRPCServer`, `server/grpc.Server`, and `server/rest.Gateway` with graceful shutdown; add/update unit tests in `cmd/worker_test.go`.

## 9. Integration Testing (`tests/integration/worker`)

- [ ] 9.1 Add testcontainers-based integration tests (PostgreSQL + OpenFGA + two local JWKS HTTP servers for STS `UserAccount` and OIDC `ServiceAccount`) in `tests/integration/worker/worker_test.go` covering both gRPC and REST transports:
  - `platform` mode: async processing + `/healthz` work; no gRPC server or `/api/v1/*` routes.
  - `standalone`/`hybrid` modes: public `/healthz` and `/api/v1/model` (JSON + indented `text/plain` `?format=dsl`); `UserAccount` allowed on `/api/v1/check` and rejected (`403`) on `/api/v1/tuples`; `ServiceAccount` allowed on `/api/v1/check` and `/api/v1/tuples` when mapped (`403` when unmapped); missing/invalid JWT (`401`); batch limit `> 50` rejected (`400`); idempotent `POST`/`DELETE /api/v1/tuples` with OpenFGA rollback on failure and `condition_name`/`condition_context` persistence when `multitenancyEnabled=true`; service-scoped paginated `GET /api/v1/tuples`; concurrent async queue processing.

## 10. Verification Suite

- [ ] 10.1 Run the full verification suite and confirm zero failures:
  - `openspec validate --all`
  - `make mocks`
  - `make build`
  - `go vet ./... && make lint` (or `golangci-lint run --new-from-rev=origin/main` if only pre-existing debt fails full lint)
  - `go test -short -race ./...`
  - `go test -v -race -tags integration ./tests/integration/worker/...`

## 11. Documentation & Rollout

- [ ] 11.1 Update `authorization-service.yaml.example`, `.env.example`, `LOCAL_DEVELOPMENT.md`, and `README.md` with `worker.mode`, `worker.service_account_jwk_set_url`, `worker.service_identities`, and the gRPC/REST API endpoints and OpenAPI references.

## Implementation Notes

- Recorded during implementation as needed.
