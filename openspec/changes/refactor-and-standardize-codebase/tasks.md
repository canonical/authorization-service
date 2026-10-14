## 1. Integration Layer Fixes and Cleanup (`internal/integration/...`)

- [x] 1.1 Fix the `ConnectTimeout` condition (`if cfg.ConnectTimeout > 0`) and delete the unused `Offset`, `PageSize`, `defaultPage`, and `defaultPageSize` declarations in `internal/integration/postgres/client.go`. Add unit tests and `testcontainers` PostgreSQL integration tests in `internal/integration/postgres/client_test.go` to verify custom `ConnectTimeout` values.
- [x] 1.2 Delete the unused stub files `internal/integration/openfga/client.go` and `internal/service/permissions/noop.go`. Run `go test ./internal/integration/openfga/... ./internal/service/permissions/...` to verify that interfaces, services, and generated mocks compile and pass unit tests.
- [x] 1.3 Move `CreateConnection` and `waitForConnectionReady` into `internal/integration/sts/client.go`. Add unit tests in `internal/integration/sts/client_test.go` for invalid addresses, connection timeouts, and client initialization.
- [x] 1.4 Replace non-context `slog` calls with `DebugContext`, `InfoContext`, `WarnContext`, and `ErrorContext` in `internal/integration/kafka/client.go`, `internal/integration/kafka/admin.go`, `internal/integration/kafka/noop.go`, `internal/integration/postgres/client.go`, `internal/integration/sts/client.go`, `internal/integration/valkey/client.go`, and `internal/integration/valkey/noop.go`. Add the missing `AGPL-3.0-only` license header to `internal/integration/kafka/noop.go`. Run unit tests across `./internal/integration/...` to verify the changes.

## 2. Configuration and Component-Scoped Initialization (`config/`)

- [x] 2.1 Remove unused `envconfig` and `default` struct tags from `config/specs.go`, update `TelemetryConfig.SetupTelemetry` to log with `InfoContext(ctx, ...)`, and update `STSConfig.CreateSTSConnection` to call `sts.CreateConnection`. Run unit tests in `config/specs_test.go` and `config/sts_test.go` to verify configuration validation and STS connection setup.
- [x] 2.2 Update `InitializeIntegrations(cfg *Config, logger *slog.Logger, tracer trace.Tracer, components ...Component)` in `config/helper.go` to initialize only the requested `Component` clients (`ComponentOpenFGA`, `ComponentOpenFGAModelWriter`, `ComponentValkey`, `ComponentPostgres`, `ComponentKafka`, and `ComponentSTS`). Add unit tests in `config/helper_test.go` for `listen`, `reaper`, `seed`, and `authz write-model` component lists.

## 3. Domain Model and Repository Standardization (`internal/model/...`, `internal/repository/...`)

- [x] 3.1 Add `CountSegments(path string) int` to `internal/model/rules/rule.go` and simplify `ParseSubject(subject string)` with `strings.Cut` in `internal/model/permissions/work.go`. Add unit tests in `internal/model/rules/rule_test.go` and `internal/model/permissions/work_test.go`.
- [x] 3.2 Define the `RuleSeedRepository` interface in `internal/repository/rules.go`, use `rules.CountSegments` in `internal/repository/rules.go`, and update `internal/repository/rules_seed.go` to use `uuid.NewV7()` and `r.metrics.ObserveQuery`. Verify these changes with unit tests in `internal/repository/rules_test.go` and `testcontainers` PostgreSQL integration tests in `tests/integration/postgres_test.go`.

## 4. Service Layer Caching, Decoupling, and Logging (`internal/logging/...`, `internal/service/...`)

- [x] 4.1 Update `TraceHandler.Handle` in `internal/logging/logging.go` to add `request_id` from `ctx` when `request_id` is not already present on the log record. Add unit tests in `internal/logging/logging_test.go`.
- [x] 4.2 Add a `sync.Map` regex cache to `internal/service/rules/resolver.go` and use `rules.CountSegments` in `internal/service/rules/pathcompile.go`. Add unit tests and benchmarks in `internal/service/rules/resolver_test.go` and `internal/service/rules/pathcompile_test.go` to verify single compilation per pattern.
- [x] 4.3 Update `RuleSeeder` in `internal/service/rules/seeder.go` to accept `repository.RuleSeedRepository`. Verify the change with unit tests in `internal/service/rules/seeder_test.go` and `testcontainers` PostgreSQL integration tests in `tests/integration/postgres_test.go`.
- [x] 4.4 Simplify cookie parsing (`extractSessionCookie` and `splitCookies`) and deduplicate request path query stripping in `internal/service/authz/external.go`. Replace non-context `slog` calls with `*Context(ctx, ...)` in `internal/service/authz/external.go`, `internal/service/permissions/service.go`, `internal/service/rules/resolver.go`, `internal/service/rules/seeder.go`, `internal/service/listen/listener.go`, `internal/service/listen/ingestor.go`, `internal/service/worker/worker.go`, `internal/service/worker/processor.go`, `internal/service/worker/reaper.go`, and `server/rest/gateway.go`. Run unit tests in `./internal/service/...` and `./server/rest/...`.

## 5. CLI Lifecycle Runner and Subcommands (`cmd/...`)

- [x] 5.1 Create the shared process runner in `cmd/runner.go` to handle `SIGINT` and `SIGTERM` signals, Prometheus metrics server startup and shutdown, and background goroutine draining. Add unit tests in `cmd/runner_test.go` for signal termination, component errors, and metrics server shutdown.
- [x] 5.2 Update `cmd/worker.go`, `cmd/listen.go`, and `cmd/reaper.go` to include `config.ComponentServer` in `LoadConfigFor`, pass their component lists to `config.InitializeIntegrations`, and call the runner in `cmd/runner.go`. Verify with unit tests in `cmd/`.
- [x] 5.3 Update `cmd/seed.go` and `cmd/authz/write-model.go` to initialize PostgreSQL and OpenFGA clients through `config.InitializeIntegrations`, add `config.ComponentServer` to `cmd/seed.go`, and use context-aware logging in `cmd/seed.go` and `cmd/serve.go`. Verify with unit tests in `cmd/` and `cmd/authz/`.
- [x] 5.4 Update `cmd/authz/check.go` to defer `tracerShutdown`, pass its component list to `config.InitializeIntegrations`, and support the optional `--token` flag (`Authorization: Bearer <token>`). Verify with unit tests in `cmd/authz/`.
- [x] 5.5 Update `cmd/migrate.go` to use `cfg.Postgres.DSN()` when the user omits `--dsn`, and avoid redundant `registry.Topics()` calls in `cmd/ensure_topics.go`. Verify with unit tests in `cmd/` and `testcontainers` PostgreSQL migration tests in `tests/integration/postgres_test.go`.

## 6. Verification Suite

- [x] 6.1 Run `go test -race ./...` and confirm zero test or race detector failures.
- [x] 6.2 Run `go test -v -tags=integration ./tests/integration/...` with `testcontainers` (PostgreSQL, Valkey, Kafka, and OpenFGA) and confirm all integration tests pass.
- [x] 6.3 Run `golangci-lint run` (or `go vet ./...`) and confirm zero diagnostic warnings.

## 7. Documentation & Rollout

- [x] 7.1 Update CLI help text and flag descriptions in `cmd/migrate.go` and `cmd/authz/check.go` for the optional `--dsn` fallback and the new `--token` flag.
- [x] 7.2 Confirm that all new and modified `.go` files start with the `AGPL-3.0-only` license header.
- [x] 7.3 Verify backward compatibility for all existing CLI commands, flags, environment variables, and container entrypoints before deployment.

## Implementation Notes

- Added `PostgresConfig.DSN()` helper on `config.PostgresConfig` in `config/specs.go` so `cmd/migrate.go` and `config/helper.go` construct PostgreSQL connection strings consistently.
- Made `InitKafkaConsumer` idempotent across duplicate `ComponentKafka` entries in `InitializeIntegrations`.
- Removed unused `internal/service/permissions/noop.go` (`NoopService`) since disabled Valkey configurations inject `valkey.NewNoopClient` into `permissions.NewService`.
- Total unit test coverage increased from `38.3%` baseline to `46.4%` (+8.1%).

