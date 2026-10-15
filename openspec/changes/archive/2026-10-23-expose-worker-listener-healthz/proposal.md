## Why

The `serve` command exposes an HTTP health check endpoint (`GET /healthz`) on the configured HTTP address (`<SERVER_HOST>:<HTTP_PORT>`, default `0.0.0.0:8070`) for container orchestrators, Kubernetes liveness/readiness probes, and Juju Pebble health checks. However, the long-running background components—`authorization-service worker` and `authorization-service listen`—only expose an optional Prometheus `/metrics` endpoint on `METRICS_PORT` (`9100`) when `METRICS_ENABLED=true` and provide no `/healthz` endpoint. Operators cannot configure consistent `/healthz` probes across all three runtime deployments, and disabling metrics leaves `worker` and `listen` without any HTTP endpoint at all.

## What Changes

- Introduce a dedicated `internal/health` package providing a shared `/healthz` handler (`health.HandleHealthz`) and standalone health check HTTP server (`health.NewServer`) that serve `GET /healthz` returning HTTP `200 OK` with `Content-Type: application/json` and JSON body `{"status":"SERVING"}` across `serve`, `worker`, and `listen`, completely decoupled from the `internal/metrics` subsystem.
- Always run the health check HTTP server in `authorization-service worker` and `authorization-service listen` on `<SERVER_HOST>:<HTTP_PORT>` (default `0.0.0.0:8070`, configured via `HTTP_PORT` / `SERVER_HTTP_PORT` / `--http-port`, matching `serve`) regardless of whether `METRICS_ENABLED` is `true` or `false`.
- Validate the `Server` configuration section in `worker` and `listen` at startup and ensure the component supervision loop gracefully shuts down the health and metrics HTTP servers and drains all background routines when any component stops or fails.
- Document the `/healthz` endpoint behavior and port configuration for `worker` and `listen` across operator and developer guides.

## Capabilities

### New Capabilities
- `component-health-checks`: Standalone HTTP health check endpoint (`GET /healthz`) contract for background runtime components (`worker` and `listen`), decoupled from the metrics subsystem.

### Modified Capabilities
- `command-config-validation`: Update `worker` and `listen` command-scoped configuration validation profiles to include the `Server` configuration section (`HTTP_PORT`, `SERVER_HOST`, `SERVER_SHUTDOWN_TIMEOUT`).

## Non-goals

- Modifying `internal/metrics` or coupling health checks to the Prometheus metrics server or `METRICS_PORT`.
- Modifying the `serve` command's gRPC health check behavior (`server/grpc`).
- Adding deep downstream dependency probes (such as active PostgreSQL, OpenFGA, or Kafka ping queries) to `/healthz`.
- Adding health check endpoints to one-shot CLI commands (`migrate`, `seed`, `reaper`, `ensure-topics`, `list-topics`, `authz`).

## Success Metrics

- `GET /healthz` returns `200 OK` with `Content-Type: application/json` and body `{"status":"SERVING"}` on `<SERVER_HOST>:<HTTP_PORT>` (default `0.0.0.0:8070`) across `serve`, `worker`, and `listen` whether `METRICS_ENABLED` is `true` or `false`.
- `internal/metrics` remains untouched and serves `/metrics` on `METRICS_PORT` (`9100`) only when `METRICS_ENABLED=true`.
- Unit and testcontainers-backed integration tests verify `/healthz` availability and graceful shutdown.

## Impact

- **Affected packages**: `internal/health` (new), `server/rest`, `cmd`, `tests/integration/worker`, `tests/integration/listener`.
- **Documentation & Configuration**: [README.md](README.md), [DEVELOPMENT.md](DEVELOPMENT.md), [LOCAL_DEVELOPMENT.md](LOCAL_DEVELOPMENT.md), [authorization-service.yaml.example](authorization-service.yaml.example).
