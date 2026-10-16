# Spec Delta

## MODIFIED Requirements

### Requirement: Command-Scoped Configuration Loading and Validation
The configuration subsystem SHALL validate only the configuration sections that the active CLI command (`serve`, `listen`, `worker`, `migrate`, `seed`, `reaper`, `topics ensure`, `topics list`, `authz write-model`, `authz check`) requires. The configuration subsystem SHALL NOT require configuration sections for unused components. The CLI SHALL keep full backward compatibility with existing flags, command aliases, and environment variables.

#### Scenario: Running listener command without OpenFGA configuration
- **WHEN** an operator runs the `listen` command with valid Server, Postgres, Kafka, Logging, Telemetry, and Metrics configuration and no OpenFGA configuration
- **THEN** the configuration validation succeeds
- **AND** the listener process starts without OpenFGA configuration errors

#### Scenario: Running serve command without Kafka configuration
- **WHEN** an operator runs the `serve` command with valid Server, ExtAuthzService, OpenFGA, Valkey, STS, Postgres, Logging, Telemetry, and Metrics configuration and no Kafka configuration
- **THEN** the configuration validation succeeds
- **AND** the server process starts without Kafka configuration errors

#### Scenario: Running worker command without Kafka configuration
- **WHEN** an operator runs the `worker` command with valid Server, Worker, OpenFGA, Postgres, Logging, Telemetry, and Metrics configuration and no Kafka configuration
- **THEN** the configuration validation succeeds
- **AND** the worker process starts without Kafka configuration errors

#### Scenario: Running migrate command with minimal required configuration
- **WHEN** an operator runs the `migrate` command with valid Postgres, Logging, and Telemetry configuration or with the `--dsn` flag
- **THEN** the configuration validation succeeds without Server, OpenFGA, Kafka, Valkey, STS, or Worker configuration

#### Scenario: Running topics ensure and topics list commands with minimal required configuration
- **WHEN** an operator runs the `topics ensure` (`ensure-topics`) or `topics list` (`list-topics`) command with valid Kafka configuration (and Logging and Telemetry configuration for `topics ensure`)
- **THEN** the configuration validation succeeds without Server, OpenFGA, Postgres, Valkey, STS, or Worker configuration

#### Scenario: Running authz write-model command without OpenFGA authorization model ID
- **WHEN** an operator runs the `authz write-model` command with a valid OpenFGA address, store ID, and API key, and without an authorization model ID
- **THEN** the configuration validation succeeds without `OPENFGA_AUTHORIZATION_MODEL_ID`

#### Scenario: Running serve command without OpenFGA authorization model ID
- **WHEN** an operator runs the `serve` command with a valid OpenFGA address, store ID, and API key, and without an authorization model ID
- **THEN** the configuration validation fails with an error that states `OPENFGA_AUTHORIZATION_MODEL_ID` is required

#### Scenario: Command validation failure for missing required section
- **WHEN** an operator runs a CLI command and a required configuration section is missing or invalid
- **THEN** the configuration validation fails with an error that identifies the missing or invalid field

### Requirement: Command-Scoped Integration Initialization
The integration subsystem SHALL initialize only the external service clients that the active CLI command requests.

#### Scenario: Initializing integrations for listener
- **WHEN** the `listen` command initializes integrations
- **THEN** the integration subsystem connects to Postgres and Kafka
- **AND** the integration subsystem does not connect to OpenFGA, STS, or Valkey

#### Scenario: Initializing integrations for serve
- **WHEN** the `serve` command initializes integrations
- **THEN** the integration subsystem connects to Postgres, OpenFGA, Valkey, and STS
- **AND** the integration subsystem does not initialize the Kafka consumer

#### Scenario: Initializing integrations for worker
- **WHEN** the `worker` command initializes integrations
- **THEN** the integration subsystem connects to Postgres and OpenFGA
- **AND** the integration subsystem does not connect to Kafka, STS, or Valkey

#### Scenario: Initializing integrations for reaper and seed
- **WHEN** the `reaper` or `seed` command initializes integrations
- **THEN** the integration subsystem connects only to Postgres
- **AND** the integration subsystem does not connect to OpenFGA, Kafka, STS, or Valkey

#### Scenario: Initializing integrations for authz write-model
- **WHEN** the `authz write-model` command initializes integrations
- **THEN** the integration subsystem initializes only the OpenFGA model-writer client without an authorization model ID
- **AND** the integration subsystem does not connect to Postgres, Kafka, STS, or Valkey
