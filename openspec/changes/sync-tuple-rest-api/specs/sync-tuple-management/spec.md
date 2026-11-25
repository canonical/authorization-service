# sync-tuple-management Specification

## Purpose

Defines the protobuf-backed gRPC and HTTP REST APIs exposed by `authorization-service worker` in `standalone` and `hybrid` modes for synchronous tuple management (`POST`/`DELETE`/`GET /api/v1/tuples`), authorization check evaluation (`POST /api/v1/check`), and OpenFGA model inspection (`GET /api/v1/model`), while preserving the asynchronous work-queue pipeline (`listen`, `worker`, `reaper`) and unauthenticated `GET /healthz`.

Goals:
- Define all new endpoints in protobuf (`api/proto/v1/tuples.proto`) so they are exposed consistently over both gRPC and `grpc-gateway` REST with automatically maintained OpenAPI definitions.
- Provide strong per-request synchronous consistency on tuple writes and deletes across PostgreSQL (`authorization_tuples`) and OpenFGA, persisting OpenFGA `condition_name` and `condition_context` (`tenant_match` from `jwt.org` when multitenancy is enabled) on both sync and async paths.
- Distinguish `UserAccount` (STS JWKS) from `ServiceAccount` (external OIDC JWKS) cryptographically via two characterizing JWKS URLs, enforcing strict M2M-only access on `/api/v1/tuples` while allowing both principal types on `/api/v1/check` and public access on `/api/v1/model`.
- Keep `serve` isolated as an Envoy `ext_authz` data-plane by hosting all new APIs on `worker`.

Non-goals:
- Removing or disabling the asynchronous `permission_update_work` loop or stale-row `reaper` in any worker mode.
- Implementing `ListObjects` or `ListUsers`.
- Accepting caller-supplied `condition` objects in `POST /api/v1/tuples` (deferred to a future extension).
- Persisting or statically seeding `service_identities` in PostgreSQL (handled declaratively via runtime configuration to align with Juju charm env-var and Pebble restart workflows).
- Adding a foreign key constraint on `authorization_tuples.service` (keeping `TEXT NOT NULL` preserves `platform` mode compatibility).

Key decisions:
- **Protobuf + gRPC + `grpc-gateway` on `worker` (`standalone` / `hybrid`)**: Reuses the repository's existing `buf`, `grpc-gateway`, and `protoc-gen-openapiv2` pipeline without altering existing `.proto` files, exposing both gRPC (`:9091`) and REST (`:8070`, alongside unauthenticated `GET /healthz`) with one shared authentication and business-logic implementation.
- **Characterizing dual-JWKS authentication (`UserAccount` vs `ServiceAccount`)**: `ext_authz_service.jwk_set_url` (STS) characterizes verified tokens as `UserAccount`; `worker.service_account_jwk_set_url` (external OIDC provider) characterizes verified tokens as `ServiceAccount`. Both URLs are required in `standalone` and `hybrid` modes, avoiding ConfigMap collisions with `serve` and eliminating claim-based heuristics.
- **Batch size limit of 50**: Caps `POST /api/v1/tuples`, `DELETE /api/v1/tuples`, and `POST /api/v1/check` at 50 items per request to bound OpenFGA latency and PostgreSQL transaction hold time.
- **OpenFGA condition mirror (`condition_name TEXT`, `condition_context JSONB`)**: Mirrors `openfga.RelationshipCondition` 1:1 on `authorization_tuples` with a SQL `CHECK (condition_context IS NULL OR condition_name IS NOT NULL)` constraint, populated whenever `multitenancyEnabled=true` (using `jwt.org` on sync writes and `Processor` on async writes).
- **PostgreSQL transaction bracketing OpenFGA `Write`**: Stages `authorization_tuples` changes in a PG transaction before calling OpenFGA `Write`, rolling back PG on any OpenFGA error and committing only after OpenFGA succeeds. If the final PG commit fails after OpenFGA succeeded, returns HTTP `500` / gRPC `Internal` advising the caller to retry (safe due to idempotency on both stores).

Backward compatibility:
- Fully backward compatible. `worker.mode` defaults to `platform`, where no gRPC server or new REST endpoints are registered, JWKS configuration is not required, and `GET /healthz`, async queue processing, and `reaper` behave identically to before.

## ADDED Requirements

### Requirement: Configurable Worker Operating Modes
The `worker` command SHALL support three configurable operating modes—`platform`, `standalone`, and `hybrid`—defaulting to `platform` for backward compatibility. The asynchronous work-queue loop, stale-row reaper (unless disabled via `--no-reaper`), and unauthenticated `GET /healthz` endpoint SHALL remain active in all modes.

#### Scenario: Running worker in default platform mode
- **WHEN** `authorization-service worker` starts with no mode configured or with mode set to `platform`
- **THEN** the worker runs the asynchronous work-queue processor, the stale-row reaper (unless `--no-reaper` is set), and the HTTP server exposing `GET /healthz`
- **AND** the worker does not start a gRPC server or register `/api/v1/tuples`, `/api/v1/check`, or `/api/v1/model` (requests to those HTTP paths return HTTP `404 Not Found`)

#### Scenario: Running worker in standalone or hybrid mode
- **WHEN** `authorization-service worker` starts with mode set to `standalone` or `hybrid`
- **THEN** the worker runs the asynchronous work-queue processor, the stale-row reaper (unless `--no-reaper` is set), the gRPC server (`<SERVER_HOST>:<GRPC_PORT>`), and the REST gateway (`<SERVER_HOST>:<HTTP_PORT>`) exposing unauthenticated `GET /healthz`, public `GET /api/v1/model`, and authenticated `/api/v1/tuples` and `/api/v1/check` endpoints

### Requirement: Dual-JWKS Characterizing Authentication and Principal Classification
Every authenticated RPC and REST endpoint (`/api/v1/tuples`, `/api/v1/check`) SHALL verify the Bearer JWT against the two characterizing JWKS endpoints and classify the caller in memory as `ServiceAccount` (`worker.service_account_jwk_set_url`, identifier from `client_id` falling back to `sub`) or `UserAccount` (`ext_authz_service.jwk_set_url`, subject from `sub`), retaining the `org` claim when present.

#### Scenario: Authenticating a ServiceAccount via service-account JWKS
- **WHEN** a client sends a request with a Bearer JWT signed by a key in `worker.service_account_jwk_set_url`
- **THEN** authentication succeeds and the caller is classified in memory as `ServiceAccount` with identifier taken from `client_id` (or `sub` if `client_id` is absent/empty)

#### Scenario: Authenticating a UserAccount via external-authz STS JWKS
- **WHEN** a client sends a request with a Bearer JWT signed by a key in `ext_authz_service.jwk_set_url`
- **THEN** authentication succeeds and the caller is classified in memory as `UserAccount` with subject taken from `sub`

#### Scenario: Rejecting missing, non-Bearer, invalid, or expired JWT
- **WHEN** a client invokes `/api/v1/tuples` or `/api/v1/check` without an `Authorization` header/metadata, without the `Bearer ` prefix, with an empty token, with a token that fails verification against both JWKS endpoints, or with a token missing its required identity claim
- **THEN** the server rejects the request with gRPC `Unauthenticated` / HTTP `401 Unauthorized` without accessing PostgreSQL or OpenFGA

#### Scenario: Rejecting UserAccount on tuple management endpoints
- **WHEN** a caller classified as `UserAccount` invokes `POST`, `DELETE`, or `GET /api/v1/tuples` (or the corresponding gRPC RPCs)
- **THEN** the server rejects the request with gRPC `PermissionDenied` / HTTP `403 Forbidden` without accessing PostgreSQL or OpenFGA

#### Scenario: Rejecting unmapped ServiceAccount on tuple management endpoints
- **WHEN** a caller classified as `ServiceAccount` invokes `POST`, `DELETE`, or `GET /api/v1/tuples` and its resolved identifier is not mapped in `worker.service_identities` (`auth_type = OIDC`)
- **THEN** the server rejects the request with gRPC `PermissionDenied` / HTTP `403 Forbidden` without accessing PostgreSQL or OpenFGA

#### Scenario: Health check and model endpoint remain unauthenticated
- **WHEN** a client sends `GET /healthz` or `GET /api/v1/model` without an `Authorization` header while the worker runs in `standalone` or `hybrid` mode
- **THEN** the server serves the request without requiring authentication

### Requirement: Synchronous Idempotent Tuple Writes
In `standalone` and `hybrid` modes, `POST /api/v1/tuples` (and its gRPC RPC) SHALL accept 1 to 50 tuples in `{"tuples": [...]}` (without caller-supplied `condition` fields), validate every tuple, and apply them synchronously and idempotently to both `authorization_tuples` (tagged with the caller's `service` and `tenant_match` from `jwt.org` when multitenancy is enabled) and OpenFGA, returning `OK` / HTTP `200 OK` only after both stores succeed.

#### Scenario: Writing a single tuple or batch up to 50 tuples synchronously
- **WHEN** a mapped `ServiceAccount` sends `POST /api/v1/tuples` with 1 to 50 valid tuples (`subject` in `type:id` or `type:id#relation` form, non-empty `relation`, `object` in `type:id` form)
- **THEN** the worker stages the inserts into `authorization_tuples` within a PostgreSQL transaction under the caller's resolved `service`, applies the writes synchronously to OpenFGA, commits the PostgreSQL transaction, and returns `OK` / HTTP `200 OK` with the count of processed tuples

#### Scenario: Persisting tenant condition when multitenancy is enabled
- **WHEN** `multitenancyEnabled` is `true` and a mapped `ServiceAccount` sends `POST /api/v1/tuples` with a valid JWT containing a non-empty `org` claim
- **THEN** the worker attaches `Condition{Name: "tenant_match", Context: {"tenant": org}}` to every OpenFGA write tuple and persists `condition_name = 'tenant_match'` and `condition_context = {"tenant": org}` in `authorization_tuples`

#### Scenario: Rejecting write when multitenancy is enabled and JWT org claim is missing
- **WHEN** `multitenancyEnabled` is `true` and a caller sends `POST /api/v1/tuples` with a JWT whose `org` claim is missing or empty
- **THEN** the server rejects the request with gRPC `Unauthenticated` / HTTP `401 Unauthorized` without modifying PostgreSQL or OpenFGA

#### Scenario: Idempotent write of an already-existing tuple
- **WHEN** a mapped `ServiceAccount` sends `POST /api/v1/tuples` containing a tuple that already exists in `authorization_tuples` and/or OpenFGA
- **THEN** duplicates are ignored on both stores and the server returns `OK` / HTTP `200 OK`

#### Scenario: Rejecting empty, oversized (>50), or malformed write payloads
- **WHEN** a mapped `ServiceAccount` sends `POST /api/v1/tuples` with 0 tuples, more than 50 tuples, or any tuple failing syntax validation
- **THEN** the server rejects the request with gRPC `InvalidArgument` / HTTP `400 Bad Request` without modifying PostgreSQL or OpenFGA

#### Scenario: Rolling back PostgreSQL when OpenFGA write fails
- **WHEN** a mapped `ServiceAccount` sends a valid `POST /api/v1/tuples` request and OpenFGA fails the write
- **THEN** the worker rolls back the PostgreSQL transaction so no partial state is committed in `authorization_tuples`
- **AND** the server returns `InvalidArgument` / HTTP `400 Bad Request` for permanent OpenFGA validation errors or `Unavailable` / HTTP `503 Service Unavailable` for transient OpenFGA failures

#### Scenario: Post-OpenFGA PostgreSQL commit failure on write
- **WHEN** OpenFGA `Write` succeeds for `POST /api/v1/tuples` but committing the PostgreSQL transaction subsequently fails
- **THEN** the server returns gRPC `Internal` / HTTP `500 Internal Server Error` with an explicit error message informing the caller that OpenFGA was updated while database persistence failed and advising the caller to retry the same request to reconcile state

### Requirement: Synchronous Idempotent Tuple Deletes
In `standalone` and `hybrid` modes, `DELETE /api/v1/tuples` (and its gRPC RPC) SHALL accept a uniform payload `{"tuples": [...]}` containing between 1 and 50 tuples, validate every tuple, and delete them synchronously and idempotently from both `authorization_tuples` in PostgreSQL and OpenFGA. The endpoint MUST return `OK` / HTTP `200 OK` only after both stores succeed.

#### Scenario: Deleting a single tuple or batch up to 50 tuples synchronously
- **WHEN** a mapped `ServiceAccount` sends `DELETE /api/v1/tuples` with 1 to 50 valid tuples
- **THEN** the worker stages the deletes from `authorization_tuples` within a PostgreSQL transaction, applies the deletes synchronously to OpenFGA, commits the PostgreSQL transaction, and returns `OK` / HTTP `200 OK` with the count of processed tuples

#### Scenario: Idempotent delete of a non-existent tuple
- **WHEN** a mapped `ServiceAccount` sends `DELETE /api/v1/tuples` containing a tuple that does not exist in `authorization_tuples` and/or OpenFGA
- **THEN** missing tuples are ignored on both stores and the server returns `OK` / HTTP `200 OK`

#### Scenario: Rejecting empty, oversized (>50), or malformed delete payloads
- **WHEN** a mapped `ServiceAccount` sends `DELETE /api/v1/tuples` with 0 tuples, more than 50 tuples, or any tuple failing syntax validation
- **THEN** the server rejects the request with gRPC `InvalidArgument` / HTTP `400 Bad Request` without modifying PostgreSQL or OpenFGA

#### Scenario: Rolling back PostgreSQL when OpenFGA delete fails
- **WHEN** a mapped `ServiceAccount` sends a valid `DELETE /api/v1/tuples` request and OpenFGA fails the delete
- **THEN** the worker rolls back the PostgreSQL transaction so `authorization_tuples` remains unchanged
- **AND** the server returns `InvalidArgument` / HTTP `400 Bad Request` for permanent OpenFGA errors or `Unavailable` / HTTP `503 Service Unavailable` for transient OpenFGA failures

#### Scenario: Post-OpenFGA PostgreSQL commit failure on delete
- **WHEN** OpenFGA delete succeeds for `DELETE /api/v1/tuples` but committing the PostgreSQL transaction subsequently fails
- **THEN** the server returns gRPC `Internal` / HTTP `500 Internal Server Error` with an explicit error message informing the caller that OpenFGA was updated while database persistence failed and advising the caller to retry the same request to reconcile state

### Requirement: Service-Scoped Paginated Tuple Listing
In `standalone` and `hybrid` modes, `GET /api/v1/tuples` (and its gRPC RPC) SHALL return a deterministically ordered, paginated list of tuples from `authorization_tuples` restricted strictly to the calling `ServiceAccount`'s resolved `service`, including persisted `condition_name` and `condition_context` when present, and supporting optional exact-match filters (`subject`, `relation`, `object`) and `page`/`page_size` pagination parameters.

#### Scenario: Listing tuples with default pagination
- **WHEN** a mapped `ServiceAccount` sends `GET /api/v1/tuples` with no query parameters
- **THEN** the server returns `OK` / HTTP `200 OK` with `tuples` belonging solely to the caller's resolved `service`, using default `page=1` and `page_size=50`, along with `page`, `page_size`, and `total_count` metadata

#### Scenario: Listing tuples with explicit page, page_size, and filters
- **WHEN** a mapped `ServiceAccount` sends `GET /api/v1/tuples?page=2&page_size=10&subject=user:alice&relation=viewer&object=doc:1` (or any subset of `subject`, `relation`, `object`, including userset subjects like `role:admin#assignee`)
- **THEN** the server returns `OK` / HTTP `200 OK` containing only tuples matching both the caller's resolved `service` and all supplied filters for the requested page, with accurate `total_count`

#### Scenario: Service isolation on listing
- **WHEN** service A and service B both have rows in `authorization_tuples` and a `ServiceAccount` mapped to service A requests `GET /api/v1/tuples`
- **THEN** the response contains only rows where `service = 'service-a'` and never exposes rows belonging to service B

#### Scenario: Rejecting invalid pagination parameters
- **WHEN** a mapped `ServiceAccount` sends `GET /api/v1/tuples` with `page < 1` or `page_size < 1` or `page_size > 100`
- **THEN** the server rejects the request with gRPC `InvalidArgument` / HTTP `400 Bad Request`

### Requirement: Authorization Check Proxy
In `standalone` and `hybrid` modes, `POST /api/v1/check` (and its gRPC RPC) SHALL accept between 1 and 50 authorization check items (`{"checks": [...]}`), authenticated for both `UserAccount` and `ServiceAccount` principals, and proxy them to OpenFGA `BatchCheck` while injecting multitenancy evaluation context (`tenant_enabled` and `user_tenant` from `jwt.org`).

#### Scenario: Evaluating one or more checks for a UserAccount or ServiceAccount
- **WHEN** an authenticated `UserAccount` or `ServiceAccount` sends `POST /api/v1/check` with 1 to 50 valid check items (each having a unique non-empty `correlation_id`, valid `subject`, non-empty `relation`, and valid `object`)
- **THEN** the worker injects `{"tenant_enabled": multitenancyEnabled, "user_tenant": jwt.org}` into each check item's OpenFGA context, executes `BatchCheck` against OpenFGA, and returns `OK` / HTTP `200 OK` mapping each `correlation_id` to its `allowed` result (or per-item error)

#### Scenario: Rejecting empty, oversized (>50), duplicate correlation_id, or malformed check requests
- **WHEN** an authenticated caller sends `POST /api/v1/check` with 0 checks, more than 50 checks, duplicate or empty `correlation_id`, or invalid tuple syntax
- **THEN** the server rejects the request with gRPC `InvalidArgument` / HTTP `400 Bad Request` without calling OpenFGA

#### Scenario: Rejecting check when multitenancy is enabled and JWT org claim is missing
- **WHEN** `multitenancyEnabled` is `true` and a caller sends `POST /api/v1/check` with a JWT whose `org` claim is missing or empty
- **THEN** the server rejects the request with gRPC `Unauthenticated` / HTTP `401 Unauthorized` without calling OpenFGA

### Requirement: Public Cached OpenFGA Authorization Model Inspection
In `standalone` and `hybrid` modes, `GET /api/v1/model` (and its gRPC RPC) SHALL publicly expose the active OpenFGA authorization model (`cfg.OpenFGA.AuthorizationModelID`) cached in memory, returning JSON (`application/json`) by default (`format=json` or omitted) and properly indented OpenFGA DSL (`text/plain; charset=utf-8`) when `format=dsl` is requested.

#### Scenario: Fetching the active authorization model as JSON
- **WHEN** any client sends `GET /api/v1/model` or `GET /api/v1/model?format=json` without an `Authorization` header
- **THEN** the server returns `OK` / HTTP `200 OK` with `Content-Type: application/json` containing the active OpenFGA authorization model (served from in-memory cache after the first fetch)

#### Scenario: Fetching the active authorization model as indented DSL
- **WHEN** any client sends `GET /api/v1/model?format=dsl` without an `Authorization` header
- **THEN** the server returns `OK` / HTTP `200 OK` with `Content-Type: text/plain; charset=utf-8` containing the properly indented OpenFGA DSL representation of the active authorization model

#### Scenario: Rejecting unsupported model format parameter
- **WHEN** any client sends `GET /api/v1/model?format=xml`
- **THEN** the server rejects the request with gRPC `InvalidArgument` / HTTP `400 Bad Request`

### Requirement: Async Worker Condition Persistence Parity
When `multitenancyEnabled` is `true`, the asynchronous `worker` pipeline (`RecordProcessed`) SHALL also persist `condition_name` (`'tenant_match'`) and `condition_context` (`{"tenant": "<tenantID>"}`) into `authorization_tuples` so that tuples written asynchronously and synchronously remain schema-consistent.

#### Scenario: Async worker persists tenant_match condition metadata
- **WHEN** `multitenancyEnabled` is `true` and the asynchronous worker processes a `permission_update_work` write operation
- **THEN** `authorization_tuples` records `condition_name = 'tenant_match'` and `condition_context = {"tenant": "<tenantID>"}` alongside the tuple
