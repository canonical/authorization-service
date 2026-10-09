## Why

Currently, the application unmarshals and validates a monolithic configuration struct (`Config`) containing settings for all system components (Server, ExtAuthz, OpenFGA, Valkey, STS, Postgres, Logging, Telemetry, Kafka, Worker, Metrics) regardless of which CLI command is executed. As a result, running lightweight or targeted components like `listen` or `migrate` fails if unneeded configuration (such as OpenFGA or gRPC server parameters) is omitted, or unnecessarily initializes unneeded client connections.

PR #80 (`fix: make openfga_authorization_model_id optional`) addressed a specific symptom where `authz write-model` failed if `OPENFGA_AUTHORIZATION_MODEL_ID` was not set. PR #80 removed `validate:"required"` from `AuthorizationModelID` in `specs.go` and added a manual runtime check in `InitializeIntegrations`.

Refactoring configuration loading and validation to be command-scoped builds on PR #80 by introducing declarative validation per CLI command. This ensures commands like `authz write-model` only require base OpenFGA credentials without `AuthorizationModelID`, while runtime commands (`serve`, `worker`, `authz check`) enforce `AuthorizationModelID` during config validation, and commands like `listen` do not require OpenFGA at all.

## What Changes

- Refactor `config.Config` and validation logic to support command-specific or component-specific configuration validation profiles.
- Update configuration loading so each CLI command (`serve`, `listen`, `worker`, `migrate`, `seed`, `reaper`, `ensure_topics`, `authz write-model`, `authz check`) specifies its required configuration modules.
- Distinguish OpenFGA requirements between model bootstrap/writing (`Address`, `StoreID`, `ApiKey`) and runtime authorization (`Address`, `StoreID`, `ApiKey`, `AuthorizationModelID`).
- Refactor integration initialization in `config/helper.go` so components only initialize required external service clients (e.g., `listen` does not initialize OpenFGA or STS; `serve` does not initialize Kafka consumer) and replace ad-hoc validation checks with declarative component validation.
- Ensure configuration validation errors clearly identify missing or invalid required options for the executing CLI command.

## Capabilities

### New Capabilities
- `command-config-validation`: CLI-command-scoped configuration loading and validation, allowing each component to validate and initialize only its required configuration sections.

### Modified Capabilities

## Impact

- `config/specs.go`, `config/viper.go`, `config/helper.go`: Modified configuration definitions, validation functions, and integration initialization functions.
- `cmd/*.go` and `cmd/authz/*.go`: Updated command entrypoints (`serve`, `listen`, `worker`, `migrate`, `seed`, `reaper`, `ensure_topics`, `authz write-model`, `authz check`) to invoke component-scoped config loading and initialization.
- CLI user experience: Commands will no longer reject execution due to missing config for unneeded components or unneeded model IDs.
