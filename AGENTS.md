# Coding Agent Instructions — Authorization Service (`AGENTS.md`)

This file describes the architecture, coding conventions, OpenSpec alignment requirements, and verification loop for coding agents working on `canonical/authorization-service`. Follow it while writing code so changes are correct the first time.

---

## 1. Project Overview & Architecture

`authorization-service` is a gRPC and REST API service for managing authorization and permissions in a microservices architecture. It provides permission management, authorization decisions via OpenFGA, Envoy external authorization (`envoy.service.auth.v3.Authorization`) for Istio/service meshes, Valkey caching, asynchronous Kafka permission updates, and STS token/session validation.

### 1.1 Layers

- **`cmd/`**: Cobra CLI entrypoints (`serve`, `listen`, `worker`, `reaper`, `seed`, `migrate`, `topics ensure`, `topics list`, `authz check`, `authz write-model`). Each command loads its config, initializes integrations, and owns its process lifecycle.
- **`config/`**: Component-scoped Viper configuration loading (`config/viper.go`), validation structs (`config/specs.go`), and wiring of integrations and services (`config/helper.go`).
- **`server/`**: gRPC server (`server/grpc/`) and `grpc-gateway` REST server (`server/rest/`), middleware, and health/metrics endpoints.
- **`internal/service/`**: Domain and application services:
  - `authz`: Envoy `ext_authz` server, session cookie and Bearer token authentication, OpenFGA batch checks.
  - `permissions`: Permission registration and work-item queueing.
  - `rules`: Route rule compilation, candidate matching, and dynamic OpenFGA tuple resolution.
  - `listen`: Kafka permission-update event ingestion and validation.
  - `worker`: Asynchronous outbox/queue processor and stale-work reaper.
- **`internal/repository/`**: PostgreSQL repositories (`jackc/pgx/v5` + `Masterminds/squirrel`).
- **`internal/integration/`**: External client wrappers and interfaces (`openfga`, `postgres`, `valkey`, `kafka`, `sts`).
- **`internal/model/`**: Plain domain types shared across layers.
- **`internal/logging/`, `internal/metrics/`, `internal/health/`**: Cross-cutting utilities usable from any layer.
- **`authz/model/`**: Embedded OpenFGA models and route rules. The core model is `core/core.fga`; each service has its own directory `services/<service>/` with a `rules.yaml` and, where needed, a `<service>.fga` module (see `authz/model/services/README.md`).
- **`migrations/`**: Embedded SQL schema migrations.
- **`openspec/`**: Spec-driven architecture proposals, design decisions, and behavioral specifications.

### 1.2 Dependency Direction

Imports flow one way:

```text
cmd → server → config → internal/service → internal/repository → internal/integration
                                     ↘              ↓
                                        internal/model
```

- `internal/` packages never import `cmd/`, `server/`, or `config/`. Pass configuration in as plain values or client-layer structs so services stay testable without Viper.
- `internal/integration/` depends only on external SDKs and generated clients (`client/`), never on `internal/service/` or `internal/repository/`. This keeps clients replaceable behind interfaces.
- `internal/repository/` does not import `internal/service/`. Services orchestrate repositories, not the reverse.
- `internal/model/` imports no other project packages, so any layer can use it without creating a cycle.
- Only `cmd/` and `config/` construct concrete clients and services. Everything else receives them through constructors, which keeps wiring in one place.

---

## 2. Coding Rules

### 2.1 One Name Per Concept

- Use one term per concept across types, functions, parameters, and OpenSpec documents. Synonyms make readers assume two concepts exist.
- Keep genuinely distinct concepts separate. For example, `config.Component` (`config/specs.go`) names a configuration section to validate, while `config.Integrations` (`config/helper.go`) holds initialized external clients. Do not use one term for the other.

### 2.2 OpenSpec Design Fidelity

- Follow the decisions in `openspec/changes/<change>/design.md` and `openspec/specs/`. Reviewers and future changes rely on the code matching the design.
- If a design specifies a single shared cache, client, or lifecycle hook, do not split it into separate instances. Split instances drift apart and give inconsistent results.
- If a change establishes or alters a convention, propose the matching `AGENTS.md` update in the same PR.

### 2.3 No Package-Level Mutable State

- Do not introduce package-level mutable state or caches (such as `var regexCache sync.Map`). Package state is invisible to constructors, shared between tests, and diverges from struct-owned state.
- Encapsulate state on structs and share it through constructors. For example, a cache of compiled rule regexes in `internal/service/rules` belongs on `RuleMatcher`, and every method that compiles a `PathRegex` must use that same cache.

### 2.4 Component-Scoped Configuration & Initialization

- Each command in `cmd/` must pass every component it depends on to `config.LoadConfigFor`. A component that is not requested is not validated, and wiring in `config/helper.go` that depends on it may silently do nothing. For example, `cmd/serve.go` and `cmd/authz/check.go` must request `config.ComponentExtAuthz`, otherwise `Integrations.jwkSetUrl` is never populated.
- When adding a `Component` or changing which config sections `config.InitializeIntegrations` depends on, update every command in `cmd/` that relies on it. Commands do not share a component list, so each one can miss the change independently.

### 2.5 Single Source of Truth & Safe Connection Strings

- `config.<Backend>Config` (Viper/validation layer) and `internal/integration/<backend>.Config` (client layer) describe the same connection parameters. Convert the config struct into the client struct, and keep connection-building logic only in the integration package (e.g., `postgres.Config.DSN()` in `internal/integration/postgres/client.go`). Duplicated helpers drift apart.
- Never build DSNs or connection URIs with unescaped `fmt.Sprintf` (e.g., `fmt.Sprintf("... password=%s ...", c.Password)`). It breaks when values contain spaces, quotes, or backslashes. Use `net/url.URL` or properly quote and escape keyword/value parameters.

### 2.6 Standard Library First

- Target the Go version in `go.mod`. Prefer standard library helpers (`slices.Contains`, `slices.IndexFunc`, `maps.Clone`, `cmp.Or`) over hand-written loops; they are shorter and already tested.
- Use standard library parsers (`http.ParseCookie`, `net/http.Request.Cookie`, `net/url`) instead of splitting headers or query strings by hand, which mishandles quoting and edge cases. For example, parse the `Cookie` header with `http.ParseCookie` rather than adding helpers like `extractSessionCookie` / `splitCookies`.

### 2.7 Consumer-Scoped Interfaces

- Define narrow interfaces where they are consumed (or in `interfaces.go` within the package). Narrow interfaces keep mocks small and dependencies explicit.
- Verify implementations with compile-time assertions, e.g. `var _ DBClientInterface = (*Client)(nil)` in `internal/integration/postgres/client.go`, so a signature change fails the build rather than a test.
- Remove dead stubs and unused interface methods. Unused methods still have to be mocked and maintained.

### 2.8 Context, Timeouts & Logging

- Use `logger.DebugContext`, `InfoContext`, `WarnContext`, and `ErrorContext` whenever a `context.Context` is in scope, so `logging.TraceHandler` can attach `trace_id` and `span_id`. For example, in `ExternalAuthzService.check` use `s.logger.DebugContext(ctx, ...)`, not `s.logger.Debug(...)`.
- Never log raw tokens, cookies, or secrets.
- Give every external network call and client initialization an explicit `context.WithTimeout` (PostgreSQL `ConnectTimeout` and initial `Ping`, STS gRPC dial, OpenFGA, Valkey, Kafka, and OpenTelemetry `tracerShutdown`). Without one, an unreachable dependency hangs the process.
- Long-running commands (`serve`, `worker`, `listen`, `reaper`) must trap `SIGINT`/`SIGTERM` and drain servers, connections, and telemetry exporters within `Server.GracefulShutdownTimeout`, so in-flight work and traces are not lost on rollout.

### 2.9 Tests & Mocks

- Write tests with the standard `testing` package; `testify` assertions are used in existing tests and are acceptable.
- Generate mocks with `go.uber.org/mock` via `//go:generate mockgen ...` directives at the top of the package's `_test.go` file, writing to the package's `mocks/` directory. Generated `mock_*.go` files are git-ignored; never commit or hand-write them.
- Integration tests live in `tests/integration/` behind the `integration` build tag.

### 2.10 Commits & Releases

- Use Conventional Commits (`feat:`, `fix:`, `docs:`, `refactor:`, `test:`, `chore:`); `release-please` derives versions and release notes from them.
- Never edit `CHANGELOG.md` by hand; `release-please` manages it.

### 2.11 License Headers

Every Go source file (excluding generated mocks/protobufs) must start with the Canonical copyright and SPDX header. Use the year the file was created; do not bump it on existing files.

```go
// Copyright 2026 Canonical Ltd.
// SPDX-License-Identifier: AGPL-3.0-only
```

### 2.12 Known Debt

Some code on `main` predates these rules and is being fixed in [#109](https://github.com/canonical/authorization-service/pull/109). Follow the rules above, not the existing pattern:

- `postgres.Config.DSN()` builds the DSN with unescaped `fmt.Sprintf` (rule 2.5).
- `internal/service/authz/external.go` parses the `Cookie` header by hand (rule 2.6).
- Some service code logs without context, e.g. `s.logger.Debug` in `ExternalAuthzService.check` (rule 2.8).

---

## 3. Verification Workflow & Commands

Before considering any task complete or opening/updating a pull request, run the full verification suite:

| Step | Command | Requirement |
| --- | --- | --- |
| **1. OpenSpec Validation** | `openspec validate --all` | All active specs and changes must validate with 0 failures. |
| **2. Regenerate Mocks** | `make mocks` (`go generate ./...`) | Generated mocks must stay in sync with interface changes. |
| **3. Build Binary** | `make build` | Must compile cleanly with zero errors. |
| **4. Static Analysis & Lint** | `go vet ./... && make lint` | Zero warnings or linter issues allowed. |
| **5. Unit Tests (Race)** | `make test` (`go test -short -race ./...`) | All unit tests must pass with the race detector enabled. |
| **6. Integration Tests** | `make test-int` (requires Docker) | Required when touching `internal/repository/`, `internal/integration/`, `migrations/`, or `cmd/`. |

---

## 4. Related Repository References

- **OpenSpec Rules & Context**: [`openspec/config.yaml`](openspec/config.yaml)
- **OpenSpec Drift Detection Skill**: [`.agents/skills/openspec-detect-drift/SKILL.md`](.agents/skills/openspec-detect-drift/SKILL.md)
- **Development & Contribution Guides**: [`DEVELOPMENT.md`](DEVELOPMENT.md), [`CONTRIBUTING.md`](CONTRIBUTING.md), [`LOCAL_DEVELOPMENT.md`](LOCAL_DEVELOPMENT.md)
