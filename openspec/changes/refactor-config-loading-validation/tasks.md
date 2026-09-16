## 1. Component Definition & Validation Framework

- [x] 1.1 Define component constants (e.g. `ComponentServer`, `ComponentOpenFGA`, `ComponentOpenFGAModelWriter`, `ComponentKafka`, `ComponentPostgres`, `ComponentValkey`, `ComponentSTS`, `ComponentWorker`, `ComponentLogging`, `ComponentTelemetry`, `ComponentMetrics`, `ComponentExtAuthz`) in `config/specs.go`.
- [x] 1.2 Implement component-scoped struct validation logic `ValidateComponents(components ...Component)` in `config/specs.go`, enforcing `AuthorizationModelID` for `ComponentOpenFGA` while omitting it for `ComponentOpenFGAModelWriter`.
- [x] 1.3 Add `LoadConfigFor(cmd *cobra.Command, components ...Component)` in `config/viper.go` to load configuration and execute component-scoped validation.

## 2. Integration Initialization Refactoring

- [x] 2.1 Update `InitializeIntegrations` in `config/helper.go` to conditionally initialize external clients (OpenFGA, Valkey, Postgres, Kafka, STS) based on required components or non-nil config sections.
- [x] 2.2 Clean up PR #80's ad-hoc `if cfg.OpenFGA.AuthorizationModelID == ""` check in `InitializeIntegrations` in favor of declarative `ComponentOpenFGA` validation.
- [x] 2.3 Ensure noop clients or non-nil defaults are safely returned when optional integrations are omitted.

## 3. Subcommand Refactoring

- [x] 3.1 Refactor `cmd/serve.go` to use `LoadConfigFor` with serve components.
- [x] 3.2 Refactor `cmd/listen.go` to use `LoadConfigFor` with listen components, and safely resolve host for metrics server.
- [x] 3.3 Refactor `cmd/worker.go` to use `LoadConfigFor` with worker components, and safely resolve host for metrics server.
- [x] 3.4 Refactor `cmd/migrate.go` to use `LoadConfigFor` with migrate components.
- [x] 3.5 Refactor `cmd/seed.go` to use `LoadConfigFor` with seed components.
- [x] 3.6 Refactor `cmd/reaper.go` to use `LoadConfigFor` with reaper components.
- [x] 3.7 Refactor `cmd/ensure_topics.go` to use `LoadConfigFor` with ensure_topics components.
- [x] 3.8 Refactor `cmd/authz/write-model.go` to use `LoadConfigFor` with `ComponentOpenFGAModelWriter`, `ComponentLogging`, `ComponentTelemetry`.
- [x] 3.9 Refactor `cmd/authz/check.go` to use `LoadConfigFor` with `authz check` components.

## 4. Verification & Testing

- [x] 4.1 Write unit tests in `config/specs_test.go` and `config/viper_test.go` verifying component-scoped validation succeeds when unneeded section configs are missing.
- [x] 4.2 Write unit tests verifying `authz write-model` config validation succeeds without `AuthorizationModelID`, whereas `serve` and `worker` fail when `AuthorizationModelID` is missing.
- [x] 4.3 Run `go test ./...` across the codebase to ensure all existing tests pass and build succeeds.
