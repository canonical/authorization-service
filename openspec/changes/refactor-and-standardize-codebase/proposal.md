# Proposal

## Why

The `authorization-service` codebase contains inconsistent patterns across its configuration, CLI, integration, service, and repository packages:
- `config.InitializeIntegrations` connects to unused services (OpenFGA, Valkey, and STS) because Viper sets default values for all configuration sections.
- CLI commands repeat the same signal handling, metrics server, and shutdown logic.
- `postgres.NewClient` has an inverted `ConnectTimeout` condition that ignores custom timeout values.
- `rules.ResourceMapper` compiles the same regular expression twice during each `Check` request.
- Service and integration log calls omit `context.Context`, so log records lack OpenTelemetry `trace_id` and `span_id` fields.

Standardizing these components improves startup isolation, runtime performance, and trace correlation while keeping all external behaviors unchanged.

## What Changes

- **Component-Scoped Integration Initialization (`config`, `cmd`)**: Update `config.InitializeIntegrations` to connect only to the `config.Component` dependencies that a command requests. Add `ComponentServer` validation to commands that read `cfg.Server`. Allow `cmd/migrate.go` to use `cfg.Postgres.DSN()` when the user omits `--dsn`.
- **Shared CLI Lifecycle Runner (`cmd`, `cmd/authz`)**: Create a shared runner for signal handling, the Prometheus metrics HTTP server, and OpenTelemetry tracer shutdown in `worker`, `listen`, `reaper`, and `authz check`. Add a `--token` flag to `authz check` for Bearer token requests.
- **Integration Fixes and Unused Code Removal (`internal/integration/postgres`, `internal/integration/openfga`, `internal/integration/sts`, `internal/service/permissions`, `config`)**: Fix the `ConnectTimeout` check in `postgres.NewClient`. Delete the unused stub files `internal/integration/openfga/client.go` and `internal/service/permissions/noop.go`, and delete unused pagination functions in `postgres/client.go`. Move STS gRPC connection helpers to `internal/integration/sts/client.go` and remove unused `envconfig` and `default` tags from `config/specs.go`.
- **Service and Repository Standardization (`internal/service/rules`, `internal/service/authz`, `internal/repository`, `internal/model`)**: Cache compiled regular expressions in `internal/service/rules/resolver.go`. Share path segment counting and subject parsing functions. Introduce a `RuleSeedRepository` interface, use `uuid.NewV7()`, and record query metrics in `rules_seed.go`. Simplify cookie parsing and remove duplicate path query stripping in `authz/external.go`.
- **Context-Aware Logging (`internal/logging`, `internal/service`, `internal/integration`, `config`, `server/rest`, `cmd`)**: Call `slog.*Context(ctx, ...)` whenever a `context.Context` is available so `logging.TraceHandler` adds `trace_id`, `span_id`, and `request_id`.

## Non-goals

- Do not change external gRPC or REST API contracts, protobuf schemas, database schemas, CLI flag names, or environment variable names.
- Do not change OpenFGA authorization models or Envoy `ext_authz` evaluation rules.

## Success Metrics

- `listen`, `reaper`, `worker`, `seed`, and `authz write-model` create zero connections to unrequested external services.
- The refactoring removes approximately 150 lines of duplicate CLI lifecycle code and 260 lines of unused integration and service code.
- All context-aware log records in `internal/service` and `internal/integration` include `trace_id` and `span_id` when an active span exists.
- `go test -race ./...` and `openspec validate` pass with zero errors.

## Capabilities

### New Capabilities

### Modified Capabilities
- `command-config-validation`: Update command-scoped configuration validation and client initialization requirements for `worker`, `reaper`, `seed`, `migrate`, `topics ensure`, `topics list`, and `authz write-model`.

## Impact

- Affected packages: `config`, `cmd`, `cmd/authz`, `server/rest`, `internal/integration/postgres`, `internal/integration/openfga`, `internal/integration/sts`, `internal/integration/kafka`, `internal/integration/valkey`, `internal/service/authz`, `internal/service/permissions`, `internal/service/rules`, `internal/service/listen`, `internal/service/worker`, `internal/repository`, `internal/model/rules`, `internal/model/permissions`, `internal/logging`.
