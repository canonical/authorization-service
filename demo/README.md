# Authorization Service — Local Demo

Minimal setup to run the Authorization Service locally alongside its infrastructure dependencies and Skaffold profiles (`istio`, `dev-setup`, `fake-webserver`).

## 1. Prepare Prerequisites

Run the preparation script from the repository root:

```bash
./demo/prepare-demo.sh
```

*(Pass `--build` to recompile `./bin/app`, or `UPDATE_K8S_BRIDGE=true` to automatically update `k8s/dev-setup/k8s-bridge.env` with your local bridge IP).*

**What to expect:**
* Starts the required Docker Compose dependencies (`postgres` on `:5433` and `openfga` on `:8082`) and waits until all containers and the `insert-hardcoded-store` job are healthy/complete (`valkey` and `kafka` are disabled for this demo).
* Applies PostgreSQL migrations, compiles and uploads the OpenFGA authorisation model, and injects the generated `authorization_model_id` into `demo/authorization-service.yaml`.
* Seeds OpenFGA tuples from `testdata/manual/tuples.yaml` and route authorisation rules into PostgreSQL (both embedded service rules and `testdata/manual/rules_seed.sql`).

## 2. Run the Application

Start the server using the demo configuration:

```bash
./demo/run-app.sh
```

**What to expect:**
* `./bin/app serve` starts in development mode with text debug logs:
  * **gRPC server (`0.0.0.0:9091`)**: serves Envoy External Authorisation (`envoy.service.auth.v3.Authorization/Check`), reachable from the Kubernetes cluster via the `ServiceEntry` defined in the `dev-setup` Skaffold profile.
  * **HTTP REST gateway (`0.0.0.0:8070`)**: serves REST endpoints and `GET /healthz`.
  * **Metrics server (`0.0.0.0:9100`)**: exposes Prometheus metrics at `GET /metrics`.

## 3. Recover After Wi-Fi / Host IP Changes

If your local network or Wi-Fi IP address changes and Kubernetes pods fail to start (`FailedCreatePodSandBox` / CNI timeout), run:

```bash
./demo/fix-network.sh
```

*(Pass `--force` to force a full restart even if the IP appears unchanged).*

**What to expect:**
* Compares the current host IPv4 address against `endpoints/kubernetes` and tests `https://10.152.183.1:443/livez`.
* If out of sync, restarts Canonical Kubernetes (`sudo snap restart k8s`), rolls out `cilium`, `istio-cni-node`, and `ztunnel`, and updates `k8s/dev-setup/k8s-bridge.env` if the bridge IP changed.
