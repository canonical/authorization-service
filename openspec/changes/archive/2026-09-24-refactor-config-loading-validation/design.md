## Context

The `authorization-service` binary provides several subcommands via Cobra: `serve`, `listen`, `worker`, `migrate`, `seed`, `reaper`, `ensure_topics`, `authz write-model`, and `authz check`.
Currently, `config.LoadConfig(cmd)` loads and validates all sections of the `Config` struct using `validator.New().Struct(c)`. Because top-level fields on `Config` were tagged with `validate:"required"`, executing any subcommand required configuration for all components. Furthermore, `config.InitializeIntegrations(cfg, ...)` eagerly creates connections for OpenFGA, Valkey, Postgres, Kafka, and STS regardless of command needs.

PR #80 (`fix: make openfga_authorization_model_id optional`) made `AuthorizationModelID` optional in `Config` validation so `authz write-model` can run before a model ID is generated, adding a manual runtime check in `InitializeIntegrations`. Rebasing on PR #80 allows us to formalize this distinction cleanly through component-scoped validation profiles.

## Goals / Non-Goals

**Goals:**
- Provide selective, command-scoped configuration validation for each CLI command.
- Differentiate OpenFGA validation requirements for runtime authorization (`ComponentOpenFGA`, requiring `AuthorizationModelID`) vs model bootstrapping (`ComponentOpenFGAModelWriter`, omitting `AuthorizationModelID`).
- Replace manual runtime checks introduced in PR #80 with declarative component-scoped validation rules.
- Ensure commands fail with descriptive validation errors when their *required* config sections are missing/invalid, but ignore unused config sections.
- Refactor integration initialization in `config/helper.go` so external clients (OpenFGA, STS, Valkey, Kafka, Postgres) are initialized conditionally based on required component flags.
- Safely update `cmd/*.go` and `cmd/authz/*.go` subcommands to specify their required component configuration and prevent nil pointer dereferences.

**Non-Goals:**
- Splitting configuration files into separate physical YAML files for each subcommand (single configuration file / environment variable source is retained).
- Changing existing environment variable names or YAML key paths.

## Decisions

### Decision 1: Introduce Component Enum / Flags and `ValidateFor`
Instead of having a static `Validate()` method that checks every field in `Config`, we introduce explicit component identifiers (e.g., `ComponentServer`, `ComponentOpenFGA`, `ComponentOpenFGAModelWriter`, `ComponentKafka`, `ComponentPostgres`, etc.) and a component-scoped validation method.

`LoadConfigFor(cmd *cobra.Command, components ...Component)` will:
1. Load defaults, environment variables, CLI flags, and configuration files into Viper as before.
2. Unmarshal into `Config`.
3. Perform validation specifically on the requested components using `ValidateComponents(components...)`.

### Decision 2: Component Configuration Mapping Matrix
Each CLI subcommand will declare its required components:
- `serve`: `ComponentServer`, `ComponentExtAuthz`, `ComponentOpenFGA` (requires model ID), `ComponentValkey`, `ComponentSTS`, `ComponentPostgres`, `ComponentLogging`, `ComponentTelemetry`, `ComponentMetrics`
- `listen`: `ComponentPostgres`, `ComponentKafka`, `ComponentLogging`, `ComponentTelemetry`, `ComponentMetrics`
- `worker`: `ComponentWorker`, `ComponentOpenFGA` (requires model ID), `ComponentPostgres`, `ComponentLogging`, `ComponentTelemetry`, `ComponentMetrics`
- `migrate`: `ComponentPostgres`, `ComponentLogging`, `ComponentTelemetry`
- `seed`: `ComponentPostgres`, `ComponentOpenFGA`, `ComponentLogging`, `ComponentTelemetry`
- `reaper`: `ComponentPostgres`, `ComponentWorker`, `ComponentLogging`, `ComponentTelemetry`, `ComponentMetrics`
- `ensure_topics`: `ComponentKafka`, `ComponentLogging`, `ComponentTelemetry`
- `authz write-model`: `ComponentOpenFGAModelWriter` (requires address/store/key, does NOT require model ID), `ComponentLogging`, `ComponentTelemetry`
- `authz check`: `ComponentPostgres`, `ComponentOpenFGA` (requires model ID), `ComponentValkey`, `ComponentSTS`, `ComponentExtAuthz`, `ComponentLogging`, `ComponentTelemetry`

### Decision 3: Modular Integration Initialization & PR #80 Cleanup
`InitializeIntegrations` will be updated to accept the list of active components or check if component configs are present/enabled before attempting connection setup.
The ad-hoc manual validation `if cfg.OpenFGA.AuthorizationModelID == ""` added in `InitializeIntegrations` by PR #80 will be removed in favor of declarative validation in `ValidateComponents` when `ComponentOpenFGA` is evaluated.

### Decision 4: Safe Host Access for Metrics
In `cmd/listen.go`, `cmd/worker.go`, and `cmd/reaper.go`, metrics server initialization will safely resolve the host IP (defaulting to `"0.0.0.0"` or `cfg.Server.Host` if `cfg.Server != nil`), avoiding nil pointer panics when `cfg.Server` is omitted.

## Risks / Trade-offs

- **[Risk]** A command author forgets to list a required component in a new command.
  → **Mitigation**: Unit tests for every CLI command ensuring `LoadConfigFor` accurately validates expected config parameters.
- **[Risk]** Rebase conflict or regression with PR #80 changes.
  → **Mitigation**: Ensure `authz write-model` unit tests and OpenFGA model ID validation tests explicitly cover both `ComponentOpenFGA` and `ComponentOpenFGAModelWriter`.
