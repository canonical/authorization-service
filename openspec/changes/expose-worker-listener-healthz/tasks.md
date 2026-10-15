## 1. Standalone Health HTTP Server (`internal/health`)

- [ ] 1.1-health-http-server Create [internal/health/http.go](internal/health/http.go) exporting `NewServer(addr string) *http.Server` that registers `GET /healthz` returning HTTP `200 OK` with `Content-Type: text/plain; charset=utf-8` and body `ok`. Create unit tests in [internal/health/http_test.go](internal/health/http_test.go) verifying: (a) `GET /healthz` returns `200 OK` with `Content-Type: text/plain; charset=utf-8` and body `ok`, (b) non-GET methods (e.g. `POST /healthz`) return `405 Method Not Allowed`, (c) unknown paths (e.g. `GET /unknown`, `GET /metrics`) return `404 Not Found`, and (d) server startup and graceful `Shutdown` over a real TCP listener.

## 2. Worker and Listener CLI Command Wiring (`cmd`)

- [ ] 2.1-worker-cmd-healthz Update `runWorker` in [cmd/worker.go](cmd/worker.go) to include `config.ComponentServer` in `config.LoadConfigFor(...)`, always start `health.NewServer(cfg.Server.GetHTTPAddress())` in its own supervised goroutine alongside the worker loop, optional reaper, and optional `metrics.NewServer(...)`, and fix the `errChan` supervision/drain loop so `healthServer` and `metricsServer` are always gracefully shut down within `cfg.Server.GracefulShutdownTimeout` and all remaining `errChan` results are drained even when a component fails. Add unit tests in [cmd/worker_test.go](cmd/worker_test.go) verifying worker flag and configuration validation (including invalid `HTTP_PORT` rejection).
- [ ] 2.2-listen-cmd-healthz Update `runListen` in [cmd/listen.go](cmd/listen.go) (and add the standard AGPL-3.0-only license header) to include `config.ComponentServer` in `config.LoadConfigFor(...)`, always start `health.NewServer(cfg.Server.GetHTTPAddress())` in its own supervised goroutine alongside the listener loop, Kafka stats poller, and optional `metrics.NewServer(...)`, and fix the `errChan` supervision/drain loop so `healthServer` and `metricsServer` are always gracefully shut down within `cfg.Server.GracefulShutdownTimeout` and all remaining `errChan` results are drained even when a component fails. Update unit tests in [cmd/listen_test.go](cmd/listen_test.go) verifying listener flag and configuration validation (including invalid `HTTP_PORT` rejection).

## 3. Testcontainers Integration Tests (`tests/integration`)

- [ ] 3.1-worker-integration-healthz Add testcontainers-backed integration tests in [tests/integration/worker/worker_test.go](tests/integration/worker/worker_test.go) running `worker.NewWorker` alongside `health.NewServer` (bound to a dynamically allocated local port) both with `metrics.NewServer` running and without `metrics.NewServer`, verifying `GET /healthz` returns `200 OK` (`ok`) while the worker processes rows from the Postgres container and stops accepting requests after graceful shutdown.
- [ ] 3.2-listener-integration-healthz Add testcontainers-backed integration tests in [tests/integration/listener/listener_test.go](tests/integration/listener/listener_test.go) running `listen.NewListener` alongside `health.NewServer` (bound to a dynamically allocated local port) both with `metrics.NewServer` running and without `metrics.NewServer`, verifying `GET /healthz` returns `200 OK` (`ok`) while the listener ingests messages from the Kafka container into Postgres and stops accepting requests after graceful shutdown.

## 4. Documentation & Rollout

- [ ] 4.1-update-docs Update [README.md](README.md), [DEVELOPMENT.md](DEVELOPMENT.md), [LOCAL_DEVELOPMENT.md](LOCAL_DEVELOPMENT.md), and [authorization-service.yaml.example](authorization-service.yaml.example) to document that `worker` and `listen` expose `GET /healthz` on `<SERVER_HOST>:<HTTP_PORT>` (default `0.0.0.0:8070`, matching `serve`) independently of `METRICS_ENABLED` and `METRICS_PORT` (`9100`), including `--http-port` overrides when running multiple components locally on the same host.

## 5. Verification Suite

- [ ] 5.1-verify-unit-tests Run `go test -race ./internal/health/... ./internal/metrics/... ./cmd/... ./config/...` and verify zero diagnostic failures.
- [ ] 5.2-verify-integration-tests Run `go test -race -tags=integration ./tests/integration/worker/... ./tests/integration/listener/...` with testcontainers and verify zero diagnostic failures.
- [ ] 5.3-verify-full-suite Run `make test` (or `go test -race ./...`) and linter checks to verify the entire repository passes cleanly.

## Implementation Notes

- Pending implementation after OpenSpec proposal review.
