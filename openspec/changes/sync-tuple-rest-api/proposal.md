# Proposal

## Why

Callers operating in `standalone` or `hybrid` deployments need synchronous APIs to write, delete, and list authorization tuples with immediate confirmation across PostgreSQL and OpenFGA, evaluate authorization checks, and inspect the active OpenFGA authorization model—without removing the existing asynchronous pipeline (`listen`, `worker`, `reaper`) or adding load to Envoy `ext_authz` (`serve`). Defining these APIs in protobuf and exposing them over both gRPC and `grpc-gateway` REST on `worker` satisfies both machine-to-machine (`ServiceAccount`) and human (`UserAccount`) callers cleanly.

## What Changes

- Add operating modes (`platform`, `standalone`, `hybrid`, default `platform`), dedicated `service_account_jwk_set_url`, and declarative `service_identities` (`OIDC`) to `WorkerConfig` and `ComponentWorker` validation. In `standalone` and `hybrid` modes, both `ext_authz_service.jwk_set_url` (`UserAccount` / STS) and `worker.service_account_jwk_set_url` (`ServiceAccount` / external OIDC) are required.
- Keep async queue processing, stale-row `reaper`, and unauthenticated `GET /healthz` active in all worker modes; in `standalone` and `hybrid`, also start the worker gRPC server and `grpc-gateway` REST gateway exposing the new protobuf-defined API (and regenerated OpenAPI specs).
- Classify every authenticated caller into an in-memory `Principal` (`UserAccount` vs `ServiceAccount`) based on which JWKS verified the Bearer JWT:
  - `POST /api/v1/tuples`, `DELETE /api/v1/tuples` (max 50 tuples, idempotent, sync PostgreSQL + OpenFGA), and `GET /api/v1/tuples` (paginated, service-scoped): `ServiceAccount` only, resolved via `worker.service_identities` (`client_id` with `sub` fallback).
  - `POST /api/v1/check` (max 50 checks, proxied to OpenFGA `BatchCheck` with multitenancy context): both `UserAccount` and `ServiceAccount`.
  - `GET /api/v1/model`: public, cached in memory, returning the active OpenFGA model as JSON (default) or indented `text/plain` DSL (`?format=dsl`).
- Add a PostgreSQL migration adding nullable `condition_name TEXT` and `condition_context JSONB` (with `CHECK (condition_context IS NULL OR condition_name IS NOT NULL)`) and service/object indexes to `authorization_tuples`, keeping `service` as `TEXT NOT NULL` (no FK). Persist `tenant_match` condition metadata from `jwt.org` (sync) and `Processor` (async) when `multitenancyEnabled=true`.

## Non-goals

- Removing or altering `listen`, async `worker` queue processing, `reaper`, or `serve` Envoy `ext_authz`.
- Implementing `ListObjects` or `ListUsers`, accepting caller-supplied `condition` fields on `POST /api/v1/tuples`, or persisting `service_identities` in PostgreSQL.

## Capabilities

### New Capabilities
- `sync-tuple-management`: Protobuf-defined gRPC and REST APIs on `worker` for synchronous tuple writes/deletes/listing, batched authorization checks, and cached model inspection, with characterizing dual-JWKS `UserAccount`/`ServiceAccount` authentication.

### Modified Capabilities
- `command-config-validation`: Mode-aware `worker` configuration (`platform`, `standalone`, `hybrid`), dual-JWKS requirement (`ext_authz_service.jwk_set_url` + `worker.service_account_jwk_set_url`), and declarative `service_identities` validation.

## Impact

- **Affected packages**: `api/proto/v1`, `api/v1`, `openapi`, `config`, `cmd`, `server/grpc`, `server/rest`, `migrations`, `internal/model/permissions`, `internal/repository`, `internal/service/worker`, `internal/metrics`, `tests/integration/worker`.
- **Backward compatibility**: `worker.mode` defaults to `platform`, requiring no configuration or behavioral changes for existing deployments.

## Success Metrics

- 100% of unit and testcontainers integration tests pass (`go test -race ./...` and `./tests/integration/worker/...`) across `platform`, `standalone`, and `hybrid` modes over both gRPC and REST.
- `UserAccount` and `ServiceAccount` principals are deterministically classified by their verifying JWKS; unauthenticated or unauthorized callers are rejected (`401`/`403`) before any DB or OpenFGA call.
- `POST`/`DELETE /api/v1/tuples` return `200 OK` only after both PostgreSQL and OpenFGA succeed, and roll back PostgreSQL cleanly on OpenFGA failure.
