# permission-update-ingestion Specification

## Purpose

Defines the permission-update ingestion modes (`platform`, `standalone`, and `hybrid`) for the listener component, enabling federated services to publish permission updates via Kafka topics, push them directly over an Ory Hydra JWT-authenticated gRPC endpoint, or operate both transports concurrently while preserving durable, idempotent persistence into the PostgreSQL work queue.

## Requirements

### Requirement: Configurable Listener Operating Modes
The `listen` command SHALL support three configurable operating modes—`platform`, `standalone`, and `hybrid`—defaulting to `platform` for backward compatibility.

#### Scenario: Running listener in default platform mode
- **WHEN** the `listen` command starts with no mode explicitly configured or with mode set to `platform`
- **THEN** the listener ensures federated Kafka topics exist (unless disabled via flag), starts the Kafka consumer and stats poller, and does not start the gRPC ingestion server

#### Scenario: Running listener in standalone mode
- **WHEN** the `listen` command starts with mode set to `standalone`
- **THEN** the listener starts the gRPC permission-update ingestion server and does not connect to Kafka, create Kafka topics, or poll Kafka statistics

#### Scenario: Running listener in hybrid mode
- **WHEN** the `listen` command starts with mode set to `hybrid`
- **THEN** the listener starts both the Kafka consumer pipeline and the gRPC permission-update ingestion server concurrently

### Requirement: Direct gRPC Permission Update Ingestion
The listener gRPC server SHALL expose a `PermissionIngestionService/IngestPermissionUpdate` RPC that validates incoming `PermissionUpdateEnvelope` messages against the federated service registry and durably persists them to the work queue.

#### Scenario: Ingesting a valid permission update over gRPC
- **WHEN** an authenticated caller invokes `IngestPermissionUpdate` with a well-formed `PermissionUpdateEnvelope` whose `service` is registered in the federated service registry
- **THEN** the listener persists the envelope in status `received` and returns `OK`

#### Scenario: Duplicate permission update over gRPC
- **WHEN** an authenticated caller invokes `IngestPermissionUpdate` with a `(service, idempotency_key)` pair that has already been ingested
- **THEN** the listener treats the request as an idempotent no-op without inserting a second row and returns `OK`

#### Scenario: Rejecting an unknown federated service over gRPC
- **WHEN** an authenticated caller invokes `IngestPermissionUpdate` with a `service` slug that is not registered in the federated service registry
- **THEN** the listener rejects the request with gRPC status `InvalidArgument` and does not persist the envelope

#### Scenario: Rejecting a malformed envelope over gRPC
- **WHEN** an authenticated caller invokes `IngestPermissionUpdate` with an empty `idempotency_key`, empty `operations`, or invalid tuple syntax
- **THEN** the listener rejects the request with gRPC status `InvalidArgument` and does not persist the envelope

### Requirement: Ory Hydra JWT Authentication for gRPC Ingestion
The listener gRPC ingestion endpoint SHALL authenticate incoming requests by verifying the Bearer JWT in the `authorization` metadata against the configured JWKS endpoint.

#### Scenario: Valid Bearer JWT provided
- **WHEN** a caller invokes `IngestPermissionUpdate` with an `authorization: Bearer <jwt>` metadata header signed by a key in the configured JWKS endpoint
- **THEN** authentication succeeds and the envelope is processed for ingestion

#### Scenario: Missing or non-Bearer authorization metadata
- **WHEN** a caller invokes `IngestPermissionUpdate` without an `authorization` metadata header or without a `Bearer ` token prefix
- **THEN** the listener rejects the request with gRPC status `Unauthenticated`

#### Scenario: Invalid or expired Bearer JWT
- **WHEN** a caller invokes `IngestPermissionUpdate` with a Bearer token whose signature verification fails or that is expired
- **THEN** the listener rejects the request with gRPC status `Unauthenticated`
