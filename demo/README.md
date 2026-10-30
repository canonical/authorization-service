# Authorization Service — Full Demo Guide

Step-by-step commands to run the full end-to-end External Authorisation demo locally with Kubernetes, Istio Ambient Mesh, Skaffold, and `./bin/app`.

---

## 0. (Optional) Recover Network After Wi-Fi / IP Changes

If your host Wi-Fi or network IP changed since Kubernetes was started, run this first so `kube-apiserver`, Cilium, Istio CNI, and `k8s/dev-setup/k8s-bridge.env` are in sync:

```bash
./demo/fix-network.sh
```

---

## 1. Start Skaffold Profiles (in 3 Separate Terminals)

Start each Skaffold profile one at a time in its own terminal, waiting for each to stabilise before starting the next:

1. **Terminal 1 — Istio Ambient Core & Gateway**:
   ```bash
   skaffold dev -p istio
   ```
   **What to expect:** Installs Gateway API CRDs, Istio Ambient components (`istiod`, `istio-cni`, `ztunnel`), the `waypoint` proxy, and `authorization-service-gateway` in the `default` namespace.

2. **Terminal 2 — Dev Setup (MeshConfig Patch & ServiceEntry)**:
   ```bash
   skaffold dev -p dev-setup
   ```
   **What to expect:** Patches Istio's `mesh` ConfigMap to register `authorization-service` as an `envoyExtAuthzGrpc` provider (`authorization-service.dev.external:9091`) and routes it to your host bridge IP (`k8s/dev-setup/k8s-bridge.env`).

3. **Terminal 3 — Fake Webserver & Authorisation Policy**:
   ```bash
   skaffold dev -p fake-webserver
   ```
   **What to expect:** Deploys `fake-webserver` (nginx), an `HTTPRoute` matching `Host: localhost` on `/api/*`, and the `CUSTOM` `AuthorizationPolicy` enforcing external authorisation on `authorization-service-gateway`.

---

## 2. Prepare Local Dependencies & Seed Data

In a working terminal at the repository root, run:

```bash
./demo/prepare-demo.sh
```

*(Pass `--build` to recompile `./bin/app`, or `UPDATE_K8S_BRIDGE=true` to overwrite `k8s/dev-setup/k8s-bridge.env`).*

**What to expect:**
* Starts Docker Compose containers for `postgres` (`:5433`) and `openfga` (`:8082`) and waits for `insert-hardcoded-store` (`01GP1254CHWJC1MNGVB0WDG1T0`) to complete (`valkey` and `kafka` remain disabled).
* Applies PostgreSQL migrations, compiles and uploads the OpenFGA authorisation model, and writes the generated `authorization_model_id` into `demo/authorization-service.yaml`.
* Seeds OpenFGA tuples from `testdata/manual/tuples.yaml` (idempotently) and seeds route authorisation rules into PostgreSQL (embedded rules + `testdata/manual/rules_seed.sql`).

---

## 3. Start the Authorization Service

Ensure the Secure Token Service (STS) is reachable on `localhost:9090` (gRPC) and `localhost:8080` (JWKS), then start the application:

```bash
./demo/run-app.sh
```

**What to expect:**
* `./bin/app serve` starts with `demo/authorization-service.yaml`:
  * **gRPC (`0.0.0.0:9091`)**: serves Envoy External Authorisation (`envoy.service.auth.v3.Authorization/Check`) for the Istio Gateway.
  * **HTTP (`0.0.0.0:8070`)**: serves REST endpoints and `GET /healthz`.
  * **Metrics (`0.0.0.0:9100`)**: serves Prometheus metrics at `GET /metrics`.

---

## 4. Execute the Demo HTTP Requests

In another terminal, update `SESSION_ID` (and `GATEWAY_IP` if needed) in `http-tests/http-client.env.json` (or pass them as environment variables), then run:

```bash
./demo/run-http-tests.sh
```

*(Press `Enter` to fire each request one by one, or pass `--no-pause` to run all four without pausing. You can also run the requests directly from GoLand / IntelliJ using `http-tests/authz-policy-test.http`).*

**What to expect (for `marco.basile@canonical.com` / `alessandro.cabbia@canonical.com`):**
1. `GET /api/v1/groups/developers` $\rightarrow$ **`200 OK`** (user has both `read` and `member` on `group:developers`).
2. `POST /api/v1/groups/developers/member` $\rightarrow$ **`403 Forbidden`** (`no_rule_matched`: singular `/member` instead of `/members`).
3. `GET /api/v1/admin/anything/can/be/here` $\rightarrow$ **`200 OK`** (user has `admin` on `platform:default`).
4. `POST /api/v1/groups/directors/members/12345` $\rightarrow$ **`200 OK`** (user has `read` on `group:directors` and `read` on `membership:12345`).
