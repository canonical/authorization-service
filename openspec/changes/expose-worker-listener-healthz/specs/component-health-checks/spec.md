## Purpose

The `component-health-checks` specification defines the observable health check contract for the long-running background components (`authorization-service worker` and `authorization-service listen`).

Container orchestrators (Kubernetes liveness and readiness probes, Juju Pebble health checks, and Docker Compose health checks) require a reliable, lightweight HTTP endpoint to verify that background worker and listener processes are alive and serving. Previously, `worker` and `listen` only started an HTTP listener when Prometheus metrics were enabled (`METRICS_ENABLED=true`) and only served `/metrics` on `METRICS_PORT` (`9100`), forcing operators to either couple health checks to Prometheus scraping or forgo HTTP health probes entirely.

Goals:
- Expose a predictable `GET /healthz` endpoint for both `worker` and `listen` on the configured HTTP server address (`<SERVER_HOST>:<HTTP_PORT>`, default `0.0.0.0:8070`, matching `serve`) returning HTTP `200 OK` with plain-text body `ok`.
- Decouple health check domain logic and server lifecycle from `internal/metrics` and `METRICS_ENABLED` so health probes remain completely independent of metrics collection.
- Maintain clean lifecycle alignment so the `/healthz` endpoint starts when the component starts and shuts down gracefully when the component terminates or when a sibling routine fails.

Key decisions:
- **Dedicated `internal/health` package on `<SERVER_HOST>:<HTTP_PORT>` (`8070`)**: Provide a standalone health HTTP server in `internal/health` bound to `<SERVER_HOST>:<HTTP_PORT>` (`HTTP_PORT` / `SERVER_HTTP_PORT` / `--http-port`, default `8070`), matching the port where `serve` exposes `/healthz`, while keeping `internal/metrics` strictly on `METRICS_PORT` (`9100`). *Alternatives considered*: (1) Reusing `internal/metrics` on `METRICS_PORT` (`9100`) and passing a `nil` registry when metrics are disabled, rejected because calling a `metrics` package with a `nil` registry just to serve health checks leaks domain boundaries and confuses future maintainers; (2) Introducing a separate `HEALTH_PORT` configuration variable, rejected because `HTTP_PORT` (`8070`) is already defined on `ServerConfig` (and registered as the persistent `--http-port` flag) and used by `serve` for `/healthz`.
- **Plain-text `ok` response body**: Return `200 OK` with `Content-Type: text/plain; charset=utf-8` and body `ok` without modifying `server/rest/gateway.go`. *Alternatives considered*: Returning JSON `{"status":"SERVING"}` to mirror `grpc-gateway`, rejected in favor of a minimal plain-text response scoped strictly to the standalone health server.

Non-goals:
- Modifying `internal/metrics` or coupling `/healthz` to `METRICS_PORT` or `METRICS_ENABLED`.
- Changing the `serve` command's REST gateway `/healthz` endpoint on `HTTP_PORT` (`8070`) or its gRPC health service.
- Performing active network round-trips to PostgreSQL, OpenFGA, or Kafka on every `/healthz` request (initialization already fails fast at startup if dependencies are unreachable, and unrecoverable runtime failures stop the component).
- Exposing health check endpoints on short-lived CLI commands (`migrate`, `seed`, `reaper`, `ensure-topics`, `list-topics`, `authz`).

Backward compatibility:
- No breaking changes. `METRICS_ENABLED`, `METRICS_PORT` (`9100`), and `METRICS_PATH` (`/metrics`) behave identically to before. `worker` and `listen` now always bind `<SERVER_HOST>:<HTTP_PORT>` (default `0.0.0.0:8070`, configurable via `HTTP_PORT`, `SERVER_HTTP_PORT`, or `--http-port`) to serve `GET /healthz`.

## ADDED Requirements

### Requirement: Worker and listener health check endpoint
When `authorization-service worker` or `authorization-service listen` is running, the component SHALL expose an HTTP health check endpoint at `GET /healthz` on the configured HTTP address (`<SERVER_HOST>:<HTTP_PORT>`, defaulting to `0.0.0.0:8070`). A `GET /healthz` request MUST return HTTP status `200 OK` with `Content-Type: text/plain; charset=utf-8` and response body `ok`.

#### Scenario: Worker health check returns 200 OK while running
- **WHEN** `authorization-service worker` is running
- **AND** an HTTP client sends `GET /healthz` to the configured HTTP address (`<SERVER_HOST>:<HTTP_PORT>`)
- **THEN** the server responds with HTTP status `200 OK`
- **AND** the response `Content-Type` header is `text/plain; charset=utf-8`
- **AND** the response body is `ok`

#### Scenario: Listener health check returns 200 OK while running
- **WHEN** `authorization-service listen` is running
- **AND** an HTTP client sends `GET /healthz` to the configured HTTP address (`<SERVER_HOST>:<HTTP_PORT>`)
- **THEN** the server responds with HTTP status `200 OK`
- **AND** the response `Content-Type` header is `text/plain; charset=utf-8`
- **AND** the response body is `ok`

#### Scenario: Non-GET method on health check endpoint is rejected
- **WHEN** `authorization-service worker` or `authorization-service listen` is running
- **AND** an HTTP client sends `POST /healthz` (or any HTTP method other than `GET` or `HEAD`) to the configured HTTP address
- **THEN** the server responds with HTTP status `405 Method Not Allowed`

#### Scenario: Unknown path on health check HTTP server returns 404
- **WHEN** `authorization-service worker` or `authorization-service listen` is running
- **AND** an HTTP client sends `GET /unknown` (or `GET /metrics`) to the health check HTTP address (`<SERVER_HOST>:<HTTP_PORT>`)
- **THEN** the server responds with HTTP status `404 Not Found`

### Requirement: Decoupling of health check server from metrics server
The `GET /healthz` endpoint on `authorization-service worker` and `authorization-service listen` MUST be served by a dedicated health HTTP server on `<SERVER_HOST>:<HTTP_PORT>` that is completely independent of `METRICS_ENABLED` and the Prometheus metrics server on `<SERVER_HOST>:<METRICS_PORT>`.

#### Scenario: Health check and metrics servers run on separate ports when metrics are enabled
- **WHEN** `authorization-service worker` or `authorization-service listen` is started with `METRICS_ENABLED=true`
- **THEN** `GET /healthz` on `<SERVER_HOST>:<HTTP_PORT>` returns HTTP status `200 OK` with body `ok`
- **AND** `GET` on `<SERVER_HOST>:<METRICS_PORT><METRICS_PATH>` returns HTTP status `200 OK` with Prometheus metrics output

#### Scenario: Health check server runs when metrics are disabled
- **WHEN** `authorization-service worker` or `authorization-service listen` is started with `METRICS_ENABLED=false`
- **THEN** `GET /healthz` on `<SERVER_HOST>:<HTTP_PORT>` returns HTTP status `200 OK` with body `ok`
- **AND** the Prometheus metrics server on `<SERVER_HOST>:<METRICS_PORT>` is not started

### Requirement: Health server port validation and lifecycle management
Both `authorization-service worker` and `authorization-service listen` MUST validate the server configuration (`HTTP_PORT` between `1` and `65535` and `SERVER_HOST` non-empty) at startup, and MUST gracefully shut down the health HTTP server within `SERVER_SHUTDOWN_TIMEOUT` when the process receives a termination signal or when another internal component fails.

#### Scenario: Invalid HTTP port fails startup validation
- **WHEN** `authorization-service worker` or `authorization-service listen` is invoked with an invalid `HTTP_PORT` or `--http-port` (such as `0`, `-1`, or `70000`)
- **THEN** the command fails fast during configuration validation with an error before starting the health HTTP server

#### Scenario: Graceful shutdown stops health check server on termination signal
- **WHEN** `authorization-service worker` or `authorization-service listen` receives `SIGINT` or `SIGTERM` (or its execution context is cancelled)
- **THEN** the health HTTP server shuts down gracefully within the configured shutdown timeout
- **AND** subsequent requests to `GET /healthz` fail to connect

#### Scenario: Sibling component failure shuts down health server cleanly
- **WHEN** `authorization-service worker` or `authorization-service listen` is running and an internal loop (such as the worker loop, reaper, listener loop, or metrics server) exits with an error
- **THEN** the command cancels the runtime context, gracefully shuts down the health HTTP server within `SERVER_SHUTDOWN_TIMEOUT`, waits for all remaining component routines to exit, and returns the component error

#### Scenario: Health server bind failure stops component cleanly
- **WHEN** `authorization-service worker` or `authorization-service listen` starts and the health HTTP server fails to bind `<SERVER_HOST>:<HTTP_PORT>` (for example, when the port is already in use)
- **THEN** the command cancels the runtime context, shuts down any other running servers, waits for all component routines to exit before cleaning up integrations, and returns a health component failure error
