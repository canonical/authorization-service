## Purpose

Updates the `command-config-validation` specification so that `listen` and `worker` CLI commands validate the `Server` configuration section (`HTTP_PORT`, `SERVER_HOST`, `SERVER_SHUTDOWN_TIMEOUT`) required by their standalone health check HTTP server (`GET /healthz`), while continuing to omit unused external service configurations (such as OpenFGA for `listen` and Kafka for `worker`).

## MODIFIED Requirements

### Requirement: Command-Scoped Configuration Loading and Validation
The configuration subsystem SHALL support command-scoped validation, allowing each CLI command (`serve`, `listen`, `worker`, `migrate`, `seed`, `reaper`, `ensure_topics`, `authz write-model`, `authz check`) to validate only its required configuration sections without requiring configuration sections for unused components.

#### Scenario: Running listener command without OpenFGA configuration
- **WHEN** the `listen` CLI command is executed with valid Server, Postgres, Kafka, Logging, Telemetry, and Metrics configuration but no OpenFGA configuration
- **THEN** configuration validation succeeds and the listener process starts without OpenFGA configuration errors

#### Scenario: Running serve command without Kafka configuration
- **WHEN** the `serve` CLI command is executed with valid Server, ExtAuthzService, OpenFGA, Valkey, STS, Postgres, Logging, Telemetry, and Metrics configuration but no Kafka configuration
- **THEN** configuration validation succeeds and the server process starts without Kafka configuration errors

#### Scenario: Running worker command without Kafka configuration
- **WHEN** the `worker` CLI command is executed with valid Server, Worker, OpenFGA, Postgres, Logging, Telemetry, and Metrics configuration but no Kafka configuration
- **THEN** configuration validation succeeds and the worker process starts without Kafka configuration errors

#### Scenario: Running migrate command with minimal required configuration
- **WHEN** the `migrate` CLI command is executed with valid Postgres, Logging, and Telemetry configuration
- **THEN** configuration validation succeeds without requiring Server, OpenFGA, Kafka, Valkey, STS, or Worker configuration

#### Scenario: Running authz write-model command without OpenFGA authorization model ID
- **WHEN** the `authz write-model` CLI command is executed with valid OpenFGA address, store ID, and API key, but no authorization model ID
- **THEN** configuration validation succeeds without requiring `OPENFGA_AUTHORIZATION_MODEL_ID`

#### Scenario: Running serve command without OpenFGA authorization model ID
- **WHEN** the `serve` CLI command is executed with valid OpenFGA address, store ID, and API key, but no authorization model ID
- **THEN** configuration validation fails with an error indicating `OPENFGA_AUTHORIZATION_MODEL_ID` is required for runtime authorization

#### Scenario: Command validation failure for missing required section
- **WHEN** a CLI command is executed and a configuration section required by that command is missing or invalid
- **THEN** configuration validation fails with an error indicating the specific missing or invalid field for that command
