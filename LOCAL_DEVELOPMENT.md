# Local Development Guide

A minimal and effective guide to setting up and running the Authorization Service from a clean slate for local development.

---

## Prerequisites

Ensure the following tools are installed on your host machine before starting:

* **Go**: Version 1.25 or later
* **Docker** & **Docker Compose**: For running containerised infrastructure dependencies
* **Make**: Build automation tool
* **curl**: Optional tool for verifying HTTP health check endpoints

---

## 1. Architecture & Logical Components

The Authorization Service is composed of three decoupled logical components, all built into the same binary (`bin/app`) and controllable via CLI subcommands:

1. **Authorization Server (`serve`)**:
   The primary API server running the gRPC server (port `9091`) and REST gateway (port `8070`). It manages service permissions, handles authorization decisions via OpenFGA/Valkey, and integrates with Envoy / Istio external authorization.
   
2. **Kafka Event Listener (`listen`)**:
   A background consumer process that subscribes to Kafka topics (`permissions.<service-slug>`). It listens for asynchronous permission update events emitted by federated services and persists them into the PostgreSQL work queue for durable processing.

3. **Async Permission Worker & Reaper (`worker`)**:
   A background processing worker that continuously polls pending permission updates from the PostgreSQL work queue and applies them to OpenFGA. It includes an embedded, in-process **reaper** process that scans for stale or timed-out processing items and reclaims them for retry.

---

## 2. Initial Project Setup

Clone the repository and prepare the local environment variables and Go modules:

```bash
# Clone the repository (if not already done)
git clone https://github.com/canonical/authorization-service.git
cd authorization-service

# Create your local environment configuration from the example file
cp .env.example .env

# ...or create your YAML configuration file from the example file
cp authorization-service.yaml.example authorization-service.yaml

# Download and tidy Go dependencies
make deps
```

---

## 3. Deploying Locally

### Option A: Recommended Quick Start (One Command)

To build the binary, spin up containerised dependencies, run database migrations, load the OpenFGA authorization model, seed route rules, and start the main server in development mode:

```bash
make dev
```

---

### Option B: Step-by-Step Execution

If you prefer to run each component manually:

#### Step 1: Build the Application Binary
```bash
make build
```
This compiles the project binary into `bin/app`.

#### Step 2: Start Infrastructure Dependencies
```bash
make start-deps
```
This starts Docker containers for **PostgreSQL** (port `5433`), **OpenFGA** (ports `8081`/`8082`), **Valkey** (port `6379`), and **Kafka** (port `9092`).

> **N.B.:** You will also need the Secure Token Service (STS) running with its gRPC server on port `9090` and HTTP server on port `8080`.

#### Step 3: Bootstrapping & Setup CLI Sequence
Before starting the `serve` process, you **must** run the following setup CLI sequence in order to properly initialise the database schema, OpenFGA model, Kafka topics, and route rules. *(Note: `make start-deps` executes this sequence automatically if `bin/app` exists).*

1. **Apply Database Migrations (`migrate`):**
   ```bash
   ./bin/app migrate --dsn "postgresql://authorization-service:password@localhost:5433/authorization-service?sslmode=disable" up
   ```
   Applies schema migrations to set up PostgreSQL database tables and the work queue.

2. **Register OpenFGA Authorization Model (`authz write-model`):**
   ```bash
   ./bin/app authz write-model 01GP1254CHWJC1MNGVB0WDG1T0 --fga-address http://localhost:8082
   ```
   Writes the authorization model DSL to OpenFGA store `01GP1254CHWJC1MNGVB0WDG1T0` and outputs the generated `OPENFGA_AUTHORIZATION_MODEL_ID`, so you have to use that value in your configuration.

3. **Ensure Kafka Topics (`ensure-topics`)** *(Optional - required if Kafka ingestion is enabled)*:
   ```bash
   ./bin/app ensure-topics
   ```
   Ensures required Kafka topics (e.g. `permissions.<slug>`) exist for federated services.

4. **Seed Route Permission Rules (`seed`):**
   ```bash
   POSTGRES_PASSWORD=password ./bin/app seed --db-port 5433 --db-user authorization-service
   ```
   Parses embedded route YAML specifications and seeds route authorization rules into PostgreSQL.

#### Step 4: Launching Service Components
Once bootstrapping is complete, you can launch the logical components:

* **Start Main Authorization Server:**
  ```bash
  ./bin/app serve
  ```
  *(Or with explicit YAML config: `./bin/app serve --config authorization-service.yaml`)*

* **Start Kafka Ingestion Listener (in a separate terminal):**
  ```bash
  ./bin/app listen
  ```

* **Start Async Permission Worker & Reaper (in a separate terminal):**
  ```bash
  ./bin/app worker
  ```

---

## 4. Local Configuration Examples

The configuration values below match the actual container parameters defined in `docker/dependencies/docker-compose.yml`.

### Example `.env` File

Copy and place this content in a `.env` file in the project root:

```env
# Server Configuration
SERVER_GRPC_PORT=9091
SERVER_HTTP_PORT=8070
SERVER_HOST=0.0.0.0
SERVER_SHUTDOWN_TIMEOUT=15s
SERVER_DEVELOPMENT=true

# External Authorization Service Configuration
EXT_AUTHZ_SERVICE_JWK_SET_URL=http://localhost:8080/.well-known/jwks.json

# OpenFGA Configuration (matches authz-openfga container)
OPENFGA_ADDRESS=http://localhost:8082
OPENFGA_STORE_ID=01GP1254CHWJC1MNGVB0WDG1T0
OPENFGA_AUTHORIZATION_MODEL_ID=
OPENFGA_API_KEY=42
OPENFGA_TIMEOUT=10s

# Valkey Configuration (matches authz-valkey container)
VALKEY_ENABLED=true
VALKEY_ADDRESS=localhost:6379
VALKEY_USERNAME=
VALKEY_PASSWORD=
VALKEY_DB=0
VALKEY_POOL_SIZE=10
VALKEY_TIMEOUT=5s
VALKEY_USE_TLS=false

# PostgreSQL Configuration (matches authz-postgres container on host port 5433)
POSTGRES_HOST=localhost
POSTGRES_PORT=5433
POSTGRES_USER=authorization-service
POSTGRES_PASSWORD=password
POSTGRES_DB_NAME=authorization-service
POSTGRES_SSL_MODE=disable
POSTGRES_MAX_OPEN_CONNS=25
POSTGRES_MAX_IDLE_CONNS=5
POSTGRES_CONN_MAX_LIFETIME=30m
POSTGRES_CONN_MAX_IDLE_TIME=5m
POSTGRES_CONNECT_TIMEOUT=10s

# STS Configuration
STS_ADDRESS=localhost:9090
STS_USE_TLS=false
STS_TIMEOUT=10s
STS_EAGER_CONNECTION_CHECK=false

# Kafka Configuration (matches authz-kafka container)
KAFKA_ENABLED=false
KAFKA_BROKERS=localhost:9092
KAFKA_CONSUMER_GROUP=authz-listener
KAFKA_TOPIC_PARTITIONS=1
KAFKA_TOPIC_REPLICATION_FACTOR=1

# Worker Configuration
WORKER_ENABLED=false
WORKER_BATCH_SIZE=100
WORKER_POLL_INTERVAL=1s
WORKER_MAX_ATTEMPTS=5
WORKER_RETRY_BACKOFF=5m
WORKER_STALE_TIMEOUT=15m
WORKER_REAPER_INTERVAL=1m

# Logging & Telemetry
LOGGING_LEVEL=debug
LOGGING_FORMAT=json
TELEMETRY_ENABLED=false
```

### Example `authorization-service.yaml` File

Alternatively, copy and place this content in `authorization-service.yaml` in the project root:

```yaml
server:
  grpc_port: 9091
  http_port: 8070
  host: "0.0.0.0"
  shutdown_timeout: 15s
  development: true

ext_authz_service:
  jwk_set_url: "http://localhost:8080/.well-known/jwks.json"

openfga:
  address: "http://localhost:8082"
  store_id: "01GP1254CHWJC1MNGVB0WDG1T0"
  authorization_model_id: ""
  api_key: "42"
  timeout: 10s

valkey:
  enabled: true
  address: "localhost:6379"
  username: ""
  password: ""
  db: 0
  pool_size: 10
  timeout: 5s
  use_tls: false

postgres:
  host: "localhost"
  port: 5433
  user: "authorization-service"
  password: "password"
  db_name: "authorization-service"
  ssl_mode: "disable"
  max_open_conns: 25
  max_idle_conns: 5
  conn_max_lifetime: 30m
  conn_max_idle_time: 5m
  connect_timeout: 10s

sts:
  address: "localhost:9090"
  use_tls: false
  timeout: 10s
  eager_connection_check: false

kafka:
  enabled: false
  brokers:
    - "localhost:9092"
  federated_services: []
  consumer_group: "authz-listener"
  topic_partitions: 1
  topic_replication_factor: 1

worker:
  enabled: false
  batch_size: 100
  poll_interval: 1s
  max_attempts: 5
  retry_backoff: 5m
  stale_timeout: 15m
  reaper_interval: 1m

logging:
  level: "debug"
  format: "json"

telemetry:
  enabled: false
  service_name: "authorization-service"
  service_version: "v1.0.0"
```

---

## 5. Service Endpoints & Verification

Once running, the following local services and endpoints are available:

| Component | Protocol / Interface | Endpoint Address |
| :--- | :--- | :--- |
| **Authorization Service gRPC** | gRPC | `localhost:9091` |
| **Authorization REST Gateway** | HTTP | `http://localhost:8070` |
| **OpenFGA HTTP API** | HTTP | `http://localhost:8082` |
| **OpenFGA gRPC API** | gRPC | `localhost:8081` |
| **OpenFGA Web UI** | HTTP / Dashboard | `http://localhost:3000` |
| **Valkey Cache** | TCP | `localhost:6379` |
| **PostgreSQL Database** | TCP | `localhost:5433` |
| **Kafka Broker** | TCP | `localhost:9092` |

### Verify Service Health

Test that the REST gateway health endpoint returns `200 OK`:

```bash
curl -i http://localhost:8070/healthz
```

---

## 6. Testing & Code Quality

Run tests and verification targets using `make`:

```bash
# Run unit tests and generate mocks
make test

# Run integration tests (requires dependencies running via 'make start-deps')
make test-int

# Generate test coverage report in HTML format
make coverage

# Run golangci-lint (requires golangci-lint installed)
make lint
```

---

## 7. Teardown & Clean Slate Reset

To stop running containers and purge build artifacts:

```bash
# Stop and remove all Docker containers and networks
make stop-deps

# Clean compiled binaries and temporary test coverage files
make clean
```

To achieve a complete clean slate (including volume data), purge Docker volumes:

```bash
docker compose -f docker/dependencies/docker-compose.yml down -v
```
