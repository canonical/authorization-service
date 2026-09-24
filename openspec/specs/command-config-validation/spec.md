# command-config-validation Specification

## Purpose
TBD - created by archiving change refactor-config-loading-validation. Update Purpose after archive.
## Requirements
### Requirement: Command-Scoped Configuration Loading and Validation
The configuration subsystem SHALL support command-scoped validation, allowing each CLI command (`serve`, `listen`, `worker`, `migrate`, `seed`, `reaper`, `ensure_topics`, `authz write-model`, `authz check`) to validate only its required configuration sections without requiring configuration sections for unused components.

#### Scenario: Running listener command without OpenFGA configuration
- **WHEN** the `listen` CLI command is executed with valid Postgres, Kafka, Logging, Telemetry, and Metrics configuration but no OpenFGA configuration
- **THEN** configuration validation succeeds and the listener process starts without OpenFGA configuration errors

#### Scenario: Running serve command without Kafka configuration
- **WHEN** the `serve` CLI command is executed with valid Server, ExtAuthzService, OpenFGA, Valkey, STS, Postgres, Logging, Telemetry, and Metrics configuration but no Kafka configuration
- **THEN** configuration validation succeeds and the server process starts without Kafka configuration errors

#### Scenario: Running worker command without Kafka configuration
- **WHEN** the `worker` CLI command is executed with valid Worker, OpenFGA, Postgres, Logging, Telemetry, and Metrics configuration but no Kafka configuration
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

### Requirement: Command-Scoped Integration Initialization
The integration subsystem SHALL initialize only the external service clients and connections required by the executing CLI command.

#### Scenario: Initializing integrations for listener
- **WHEN** integrations are initialized for the `listen` command
- **THEN** Postgres and Kafka integrations are initialized, while OpenFGA, STS, and Valkey client connections are omitted or not required

#### Scenario: Initializing integrations for serve
- **WHEN** integrations are initialized for the `serve` command
- **THEN** Postgres, OpenFGA, Valkey, and STS integrations are initialized, while Kafka consumer initialization is omitted

