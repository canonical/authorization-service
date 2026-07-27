<p align="center">
  <img
    src="cerberus-logo.png"
    alt="Permissions Updater Logo"
    width="1200"
  />
</p>

# Authorization Service
A production-grade gRPC and REST API service for managing authorization and permissions in a microservices architecture. Built with Go 1.25, leveraging OpenFGA for fine-grained authorization, Valkey for caching, and PostgreSQL for persistent queue-based permission-update operations.

## Overview
The Authorization Service (codename Cerberus) is designed to provide:
1. **Permission Management**: Register and manage service permissions via a versioned gRPC API
2. **Authorization Decisions**: Perform authorization decisions using OpenFGA
3. **Envoy Integration**: Implements Envoy's standard External Authorization API (v3) for use with Istio or other service meshes
4. **Caching**: Uses Valkey for high-performance authorization decision caching
5. **Ingestion Pipeline**: Highly scalable, async Kafka-based permission-update event ingestion
6. **Token Validation**: Integrates with the Secure Token Service (STS) for token validation

## Architecture
```mermaid
graph TB
    subgraph clients["Clients"]
        A["Microservices"]
        B["Envoy/Istio"]
        C["External Apps"]
    end
    subgraph authz["Authorization Service"]
        REST["REST API Gateway<br/>:8070"]
        GRPC["gRPC Server<br/>:9091"]
        PERMS["Permissions<br/>Service"]
        AUTHZ["Authorization<br/>Service"]
        REST -->|Transcode| GRPC
        GRPC --> PERMS
        GRPC -->|External AuthZ| AUTHZ
    end
    subgraph integrations["Integrations"]
        FGA["OpenFGA<br/>Fine-Grained<br/>Authorization"]
        VALKEY["Valkey<br/>Cache"]
        STS["Secure Token<br/>Service"]
        POSTGRES["Postgres DB<br/>Queue/State"]
        KAFKA["Kafka Broker"]
    end
    subgraph external["External Services"]
        ISTIO["Istio Service Mesh"]
    end
    A -->|Register Permissions| GRPC
    B -->|External AuthZ Check| GRPC
    C -->|REST Health Checks| REST
    PERMS --> VALKEY
    AUTHZ --> FGA
    AUTHZ --> VALKEY
    AUTHZ --> STS
    ISTIO -->|Authorization Requests| GRPC
    KAFKA -->|Permission Updates| GRPC
    POSTGRES -->|Durability Queue| GRPC
```

## Features
- **Cobra CLI**: Professional command-line interface with automatic help generation and dedicated subcommands
- **Versioned gRPC APIs** with Go code generation from protobuf definitions
- **REST API Gateway** using grpc-gateway for transcoding gRPC to REST (currently exposes health endpoints)
- **OpenFGA Integration**: Fine-grained, relationship-based access control
- **Valkey Caching**: Cache authorization decisions for ultra-low latency check responses
- **Database Durability**: PostgreSQL-backed transactional queue for robust, horizontally scalable ingestion
- **Kafka-Based Ingestion**: Listeners that consume permission changes asynchronously from federated services
- **Graceful Shutdown**: Strict connection cleanup and signal handling

## Prerequisites
- **Go**: 1.25 or later
- **Docker**: For running dependencies
- **Docker Compose**: For orchestrating containers

## Installation & Setup

### 1. Clone the Repository
```bash
git clone https://github.com/canonical/authorization-service.git
cd authorization-service
```

### 2. Install Dependencies
```bash
go mod download
go mod tidy
```

### 3. Build the Service
```bash
make build
```
This creates `bin/app` binary.
## Running the Service

### Quick Start (Recommended)
```bash
# Start Docker dependencies (Postgres, Kafka, Valkey, OpenFGA, STS)
make start-deps

# Run the service (in another terminal)
./bin/app serve
```

### Development Mode
Builds, starts dependencies, and runs with debug logging:
```bash
make dev
```

### Using Make
```bash
make build        # Build binary (bin/app)
make run          # Run service
make start-deps   # Start docker dependencies
make stop-deps    # Stop docker dependencies
make test         # Run unit tests
make test-int     # Run integration tests
make coverage     # Generate HTML coverage report
```

---

## CLI Commands
The service uses [Cobra](https://github.com/spf13/cobra) to expose multiple operational subcommands:

### `serve` - Start the Service
```bash
./bin/app serve
```
Starts both the gRPC server (default port `9091`) and the REST gateway (default port `8070`).

### `migrate` - Run Database Migrations
```bash
./bin/app migrate --dsn "postgres://authz:authz-password@localhost:5432/cerberus?sslmode=disable" up
```
Runs schema migrations against the PostgreSQL database. Supports `up`, `down [version]`, and `status` actions.

### `listen` - Start Kafka Listener
```bash
./bin/app listen
```
Starts the Kafka permission-update listener which consumes permission events from federated services' `<slug>.permissions` topics and persists them into the PostgreSQL work queue.

### `worker` - Start Async Permission Worker
```bash
./bin/app worker
```
Starts the background worker that polls the PostgreSQL work queue and applies the pending permission changes to OpenFGA.

### `reaper` - One-off Stale-Row Reaper
```bash
./bin/app reaper
```
Scans the work queue for processing jobs that have timed out and returns them to the queue to be retried.

### `authz` - Authorization Model Management
Provides utilities for checking permissions and writing models to OpenFGA directly:
```bash
./bin/app authz check <subject> <relation> <object>
./bin/app authz write-model <file-path>
```

### `version` - Show Version
```bash
./bin/app version
```

---

## Configuration
Configuration is managed through **Viper**, providing a robust, multi-layered system. Configuration can be passed via:
1. **Cobra CLI Flags** (highest priority)
2. **Environment Variables** (nested paths mapped to structured uppercase words)
3. **YAML Config File** (loaded from `./cerberus.yaml`, `/etc/authz/cerberus.yaml`, or specified with `-c` / `--config`)
4. **Go-defined Defaults** (lowest priority)

To view or start with a baseline YAML configuration, refer to the provided [cerberus.yaml.example](cerberus.yaml.example) file.

### CLI Configuration Flags
The following flags are available globally across all CLI commands to quickly override settings:
* `-c, --config <path>`: Path to a custom YAML configuration file.
* `--grpc-port <int>`: Overrides the gRPC server port.
* `--http-port <int>`: Overrides the HTTP API Gateway port.
* `--server-host <string>`: Overrides the server bind host address.
* `--db-host <string>`: Overrides the PostgreSQL database host.
* `--db-port <int>`: Overrides the PostgreSQL database port.
* `--db-name <string>`: Overrides the PostgreSQL database name.
* `--db-user <string>`: Overrides the PostgreSQL database user.
* `--dev`: Enables development mode.
* `--log-level <string>`: Overrides the logging level.
* `--log-format <string>`: Overrides the logging format (`json` or `text`).
* `--fga-address <string>`: Overrides the OpenFGA server address.
* `--fga-store-id <string>`: Overrides the OpenFGA Store ID.
* `--fga-model-id <string>`: Overrides the OpenFGA Authorization Model ID.

### Structured Environment Variables & YAML Keys

#### Server Configuration
| YAML Path | Environment Variable | Type | Default | Description |
|-----------|----------------------|------|---------|-------------|
| `server.grpc_port` | `SERVER_GRPC_PORT` | int | `9091` | gRPC server port |
| `server.http_port` | `SERVER_HTTP_PORT` | int | `8070` | REST gateway port |
| `server.host` | `SERVER_HOST` | string | `0.0.0.0` | Server bind address |
| `server.shutdown_timeout` | `SERVER_SHUTDOWN_TIMEOUT` | duration | `15s` | Graceful shutdown timeout |
| `server.development` | `SERVER_DEVELOPMENT` | bool | `false` | Enable development mode (e.g. gRPC reflection) |

#### External Authz Service Configuration
| YAML Path | Environment Variable | Type | Default | Description |
|-----------|----------------------|------|---------|-------------|
| `ext_authz_service.jwk_set_url` | `EXT_AUTHZ_SERVICE_JWK_SET_URL` | string | `http://localhost:8080/.well-known/jwks.json` | JWKS URL to verify external JWT tokens |

#### OpenFGA Configuration
| YAML Path | Environment Variable | Type | Default | Description |
|-----------|----------------------|------|---------|-------------|
| `open_fga.address` | `OPEN_FGA_ADDRESS` | string | `http://localhost:8081` | OpenFGA server address |
| `open_fga.store_id` | `OPEN_FGA_STORE_ID` | string | *(Required)* | OpenFGA store ID |
| `open_fga.authorization_model_id`| `OPEN_FGA_AUTHORIZATION_MODEL_ID` | string | *(Required)* | OpenFGA authorization model ID |
| `open_fga.api_key` | `OPEN_FGA_API_KEY` | string | *(Required)* | OpenFGA authentication API token |
| `open_fga.timeout` | `OPEN_FGA_TIMEOUT` | duration | `10s` | Request timeout |

#### Valkey Configuration
| YAML Path | Environment Variable | Type | Default | Description |
|-----------|----------------------|------|---------|-------------|
| `valkey.enabled` | `VALKEY_ENABLED` | bool | `false` | Enable Valkey caching |
| `valkey.address` | `VALKEY_ADDRESS` | string | `localhost:6379` | Valkey server address (Required if enabled) |
| `valkey.username` | `VALKEY_USERNAME` | string | `""` | Valkey username |
| `valkey.password` | `VALKEY_PASSWORD` | string | `""` | Valkey password |
| `valkey.db` | `VALKEY_DB` | int | `0` | Database index |
| `valkey.pool_size` | `VALKEY_POOL_SIZE` | int | `10` | Max connection pool size |
| `valkey.timeout` | `VALKEY_TIMEOUT` | duration | `5s` | Read/write timeout |
| `valkey.use_tls` | `VALKEY_USE_TLS` | bool | `false` | Establish secure TLS connection |

#### Postgres Configuration
| YAML Path | Environment Variable | Type | Default | Description |
|-----------|----------------------|------|---------|-------------|
| `postgres.host` | `POSTGRES_HOST` | string | `localhost` | Database host |
| `postgres.port` | `POSTGRES_PORT` | int | `5432` | Database port |
| `postgres.user` | `POSTGRES_USER` | string | `authz` | Database user |
| `postgres.password` | `POSTGRES_PASSWORD` | string | `authz-password` | Database password |
| `postgres.db_name` | `POSTGRES_DB_NAME` | string | `cerberus` | Database name |
| `postgres.ssl_mode` | `POSTGRES_SSL_MODE` | string | `disable` | SSL mode: `disable`, `require`, `verify-ca`, `verify-full` |
| `postgres.max_open_conns` | `POSTGRES_MAX_OPEN_CONNS` | int | `25` | Maximum open database connections |
| `postgres.max_idle_conns` | `POSTGRES_MAX_IDLE_CONNS` | int | `5` | Maximum idle database connections |
| `postgres.conn_max_lifetime` | `POSTGRES_CONN_MAX_LIFETIME` | duration | `30m` | Maximum connection lifetime |
| `postgres.conn_max_idle_time` | `POSTGRES_CONN_MAX_IDLE_TIME` | duration | `5m` | Maximum idle connection lifetime |
| `postgres.connect_timeout` | `POSTGRES_CONNECT_TIMEOUT` | duration | `10s` | Connection timeout |

#### STS Configuration
| YAML Path | Environment Variable | Type | Default | Description |
|-----------|----------------------|------|---------|-------------|
| `sts.address` | `STS_ADDRESS` | string | `localhost:9090` | Secure Token Service server address |
| `sts.use_tls` | `STS_USE_TLS` | bool | `false` | Establish secure TLS connection to STS |
| `sts.timeout` | `STS_TIMEOUT` | duration | `10s` | Request timeout |
| `sts.eager_connection_check` | `STS_EAGER_CONNECTION_CHECK` | bool | `false` | Block startup until connection to STS is ready |

#### Kafka Configuration
| YAML Path | Environment Variable | Type | Default | Description |
|-----------|----------------------|------|---------|-------------|
| `kafka.enabled` | `KAFKA_ENABLED` | bool | `false` | Enable Kafka listener |
| `kafka.brokers` | `KAFKA_BROKERS` | []string | `localhost:9092` | Kafka broker addresses (Required if enabled) |
| `kafka.federated_services` | `KAFKA_FEDERATED_SERVICES` | []string | *(Required if enabled)* | Service slugs whose permission topics are to be federated |
| `kafka.consumer_group` | `KAFKA_CONSUMER_GROUP` | string | `authz-listener` | Kafka consumer group ID |
| `kafka.topic_partitions` | `KAFKA_TOPIC_PARTITIONS` | int | `1` | Default partition count for auto-created topics |
| `kafka.topic_replication_factor` | `KAFKA_TOPIC_REPLICATION_FACTOR` | int | `1` | Default replication factor for auto-created topics |

#### Worker Configuration
| YAML Path | Environment Variable | Type | Default | Description |
|-----------|----------------------|------|---------|-------------|
| `worker.enabled` | `WORKER_ENABLED` | bool | `false` | Enable the permission update background worker |
| `worker.batch_size` | `WORKER_BATCH_SIZE` | int | `100` | Processing batch size from work table |
| `worker.poll_interval` | `WORKER_POLL_INTERVAL` | duration | `1s` | Ingestion poll interval |
| `worker.max_attempts` | `WORKER_MAX_ATTEMPTS` | int | `5` | Maximum retries before a row is marked failed |
| `worker.retry_backoff` | `WORKER_RETRY_BACKOFF` | duration | `5m` | Delay before a failed row can be claimed again |
| `worker.stale_timeout` | `WORKER_STALE_TIMEOUT` | duration | `15m` | Max duration a row can sit in processing state before reclamation |
| `worker.reaper_interval` | `WORKER_REAPER_INTERVAL` | duration | `1m` | Execution interval of the in-process worker reaper |

#### Logging Configuration
| YAML Path | Environment Variable | Type | Default | Description |
|-----------|----------------------|------|---------|-------------|
| `logging.level` | `LOGGING_LEVEL` | string | `info` | Log level: `debug`, `info`, `warn`, `error` |
| `logging.format` | `LOGGING_FORMAT` | string | `json` | Log output format: `json` or `text` |

#### Telemetry Configuration
| YAML Path | Environment Variable | Type | Default | Description |
|-----------|----------------------|------|---------|-------------|
| `telemetry.enabled` | `TELEMETRY_ENABLED` | bool | `false` | Enable OTel tracer |
| `telemetry.otlp_endpoint` | `TELEMETRY_OTLP_ENDPOINT`| string | `""` | OTLP gRPC endpoint |
| `telemetry.service_name` | `TELEMETRY_SERVICE_NAME` | string | `authorization-service` | Service name in exported traces |
| `telemetry.service_version` | `TELEMETRY_SERVICE_VERSION` | string | `v1.0.0` | Service version in exported traces |

---

## Running Tests

### Unit Tests
```bash
make test
```

### Integration Tests
Requires running Docker environment:
```bash
make test-int
```

### Coverage Report
Generates code coverage maps:
```bash
make coverage
```
View the results by opening `coverage.html` in your browser.

---

## API Endpoints

### gRPC Service Endpoints (Port `9091`)
- **Permissions Management Service**: `authorization.service.api.v1.PermissionsService`
  - `RegisterPermissions`
  - `GetPermissions`
  - `ListPermissions`
  - `DeletePermissions`
- **Envoy External Authorization**: `envoy.service.auth.v3.Authorization`
  - `Check`

### REST Gateway Endpoints (Port `8070`)
- `GET /healthz` - Liveness/readiness health check
*Note: The `/v1/permissions` routes are defined in proto definitions but are currently comment-disabled in `gateway.go` and require direct gRPC client calls.*

---

## Service Federation
Service onboarding and federation into Cerberus are managed through a dedicated pull request workflow, treating the PR as the primary source of truth for configuration and metadata.

If you are a team onboarding your service, you can open your pull request with the dedicated federation template pre-loaded by clicking the button below:

<p align="center">
  <a href="https://github.com/canonical/authorization-service/compare/main...?expand=1&template=federation_onboarding.md">
    <img src="https://img.shields.io/badge/Onboard_Service-Federation_PR_Template-0066cc?style=for-the-badge&logo=github&logoColor=white" alt="Onboard Service with Federation PR Template" />
  </a>
</p>

Alternatively, you can manually select the `federation_onboarding.md` template when opening your Pull Request.

## Contributing
See [CONTRIBUTING.md](CONTRIBUTING.md) for guidelines.

## License
See [LICENSE](LICENSE) file for details.
