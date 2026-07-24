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
Configuration is managed through environment variables using the `envconfig` library.

### Server Configuration
| Variable | Type | Default | Description |
|----------|------|---------|-------------|
| `GRPC_PORT` | int | `9091` | gRPC server port |
| `HTTP_PORT` | int | `8070` | REST gateway port |
| `SERVER_HOST` | string | `0.0.0.0` | Server bind address |
| `SERVER_SHUTDOWN_TIMEOUT` | duration | `15s` | Graceful shutdown timeout |
| `DEV` | bool | `false` | Enable development mode (e.g. gRPC reflection) |

### External Authz Service Configuration
| Variable | Type | Default | Description |
|----------|------|---------|-------------|
| `EXTAUTHZ_JWK_SET_URL` | string | `http://localhost:8080/.well-known/jwks.json` | JWKS URL to verify external JWT tokens |

### OpenFGA Configuration
| Variable | Type | Default | Description |
|----------|------|---------|-------------|
| `OPENFGA_ADDRESS` | string | `http://localhost:8081` | OpenFGA server address |
| `OPENFGA_STORE_ID` | string | *(Required)* | OpenFGA store ID |
| `OPENFGA_AUTHZ_MODEL_ID` | string | *(Required)* | OpenFGA authorization model ID |
| `OPENFGA_API_KEY` | string | *(Required)* | OpenFGA authentication API token |
| `OPENFGA_TIMEOUT` | duration | `10s` | Request timeout |

### Valkey Configuration
| Variable | Type | Default | Description |
|----------|------|---------|-------------|
| `VALKEY_ENABLED` | bool | `false` | Enable Valkey caching |
| `VALKEY_ADDRESS` | string | `localhost:6379` | Valkey server address (Required if enabled) |
| `VALKEY_USERNAME` | string | `""` | Valkey username |
| `VALKEY_PASSWORD` | string | `""` | Valkey password |
| `VALKEY_DB` | int | `0` | Database index |
| `VALKEY_POOL_SIZE` | int | `10` | Max connection pool size |
| `VALKEY_TIMEOUT` | duration | `5s` | Read/write timeout |
| `VALKEY_USE_TLS` | bool | `false` | Establish secure TLS connection |

### Postgres Configuration
| Variable | Type | Default | Description |
|----------|------|---------|-------------|
| `POSTGRES_HOST` | string | `localhost` | Database host |
| `POSTGRES_PORT` | int | `5432` | Database port |
| `POSTGRES_USER` | string | `authz` | Database user |
| `POSTGRES_PASSWORD` | string | `authz-password` | Database password |
| `POSTGRES_DB` | string | `cerberus` | Database name |
| `POSTGRES_SSL_MODE` | string | `disable` | SSL mode: `disable`, `require`, `verify-ca`, `verify-full` |
| `POSTGRES_MAX_OPEN_CONNS` | int | `25` | Maximum open database connections |
| `POSTGRES_MAX_IDLE_CONNS` | int | `5` | Maximum idle database connections |
| `POSTGRES_CONN_MAX_LIFETIME` | duration | `30m` | Maximum connection lifetime |
| `POSTGRES_CONN_MAX_IDLE_TIME` | duration | `5m` | Maximum idle connection lifetime |
| `POSTGRES_CONNECT_TIMEOUT` | duration | `10s` | Connection timeout |

### STS Configuration
| Variable | Type | Default | Description |
|----------|------|---------|-------------|
| `STS_ADDRESS` | string | `localhost:9090` | Secure Token Service server address |
| `STS_USE_TLS` | bool | `false` | Establish secure TLS connection to STS |
| `STS_TIMEOUT` | duration | `10s` | Request timeout |
| `STS_EAGER_CONNECTION_CHECK` | bool | `false` | Block startup until connection to STS is ready |

### Kafka Configuration
| Variable | Type | Default | Description |
|----------|------|---------|-------------|
| `KAFKA_ENABLED` | bool | `false` | Enable Kafka listener |
| `KAFKA_BROKERS` | []string | `localhost:9092` | Kafka broker addresses (Required if enabled) |
| `FEDERATED_SERVICES` | []string | *(Required if enabled)* | Service slugs whose permission topics are to be federated |
| `KAFKA_CONSUMER_GROUP` | string | `authz-listener` | Kafka consumer group ID |
| `KAFKA_TOPIC_PARTITIONS` | int | `1` | Default partition count for auto-created topics |
| `KAFKA_TOPIC_REPLICATION_FACTOR` | int | `1` | Default replication factor for auto-created topics |

### Worker Configuration
| Variable | Type | Default | Description |
|----------|------|---------|-------------|
| `WORKER_ENABLED` | bool | `false` | Enable the permission update background worker |
| `WORKER_BATCH_SIZE` | int | `100` | Processing batch size from work table |
| `WORKER_POLL_INTERVAL` | duration | `1s` | Ingestion poll interval |
| `WORKER_MAX_ATTEMPTS` | int | `5` | Maximum retries before a row is marked failed |
| `WORKER_RETRY_BACKOFF` | duration | `5m` | Delay before a failed row can be claimed again |
| `WORKER_STALE_TIMEOUT` | duration | `15m` | Max duration a row can sit in processing state before reclamation |
| `WORKER_REAPER_INTERVAL` | duration | `1m` | Execution interval of the in-process worker reaper |

### Logging Configuration
| Variable | Type | Default | Description |
|----------|------|---------|-------------|
| `LOG_LEVEL` | string | `info` | Log level: `debug`, `info`, `warn`, `error` |
| `LOG_FORMAT` | string | `json` | Log output format: `json` or `text` |

### Telemetry Configuration
| Variable | Type | Default | Description |
|----------|------|---------|-------------|
| `TELEMETRY_ENABLED` | bool | `false` | Enable OTel tracer |
| `OTEL_EXPORTER_OTLP_ENDPOINT`| string | `""` | OTLP gRPC endpoint |
| `OTEL_SERVICE_NAME` | string | `authorization-service` | Service name in exported traces |
| `OTEL_SERVICE_VERSION` | string | `v1.0.0` | Service version in exported traces |

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
