# Development Guide
This guide covers how to develop and work with the Authorization Service codebase.
## Architecture Overview
The Authorization Service follows a layered architecture:
```
CLI Layer (Cobra)
        ↓
Configuration Layer (envconfig)
        ↓
Service Layer (Business Logic)
        ↓
Integration Layer (External Services)
        ↓
API Layer (gRPC + REST)
```
### CLI Layer
Located in `cmd/`:
- **root.go**: Defines the root command and command registration
- **serve.go**: Main serve command with configuration loading via envconfig
- **version.go**: Version command
### Configuration Layer
Configuration is loaded in `cmd/serve.go` using:
- **envconfig**: Environment variable parsing with defaults
- **Struct tags**: Define variable names, types, and defaults
- **Direct instantiation**: No separate config package (avoids circular imports)
### Service Layer
Located in `internal/service/`:
- **permissions/**: Permission registration and management
- **authz/**: Authorization checks and decision logic
### Integration Layer
Located in `internal/integrations/`:
- **openfga/**: Client for fine-grained authorization
- **valkey/**: Client for caching
- **sts/**: Client for secure token service
### API Layer
Located in `internal/server/`:
- **grpc/**: gRPC server implementation
- **rest/**: REST gateway using grpc-gateway
## Key Design Patterns
### Dependency Injection
Services receive their dependencies via constructor functions:
```go
func NewService(cache *valkey.Client, logger *slog.Logger) *Service {
    return &Service{
        cache:  cache,
        logger: logger,
    }
}
```
### Interface-Based Design
Integration clients use interfaces for mockability:
```go
type Client interface {
    Check(ctx context.Context, req *CheckRequest) (*CheckResponse, error)
    Write(ctx context.Context, req *WriteRequest) (*WriteResponse, error)
    Close() error
}
```
### Error Handling
Errors are wrapped with context:
```go
if err != nil {
    return nil, fmt.Errorf("failed to create client: %w", err)
}
```
### Logging
Structured JSON logging using `log/slog`:
```go
logger.Info("Starting service", "version", Version, "port", cfg.Server.GRPCPort)
logger.Error("Connection failed", "error", err, "service", "OpenFGA")
```
## Configuration Management
### Using envconfig
Configuration is defined as struct fields with tags:
```go
type ServerConfig struct {
    GRPCPort int           `envconfig:"GRPC_PORT" default:"9090"`
    Host     string        `envconfig:"SERVER_HOST" default:"0.0.0.0"`
    Timeout  time.Duration `envconfig:"SERVER_SHUTDOWN_TIMEOUT" default:"30s"`
}
```
Loading configuration:
```go
cfg := &Config{}
if err := envconfig.Process("", cfg); err != nil {
    return fmt.Errorf("failed to load configuration: %w", err)
}
```
### Adding New Configuration
1. Add field to appropriate config struct in `cmd/serve.go`
2. Add envconfig tag with environment variable name
3. Add default tag with sensible default
4. Update README.md with the new variable
Example:
```go
type MyConfig struct {
    MyVar string `envconfig:"MY_VAR" default:"default-value"`
}
```
Then document in README.md Configuration section.
## Development Workflow
### 1. Local Setup
```bash
# Install dependencies
go mod download
# Build binary
make build
# Start dependencies
make start-deps
```

### Start istio infra for external authorization
Refer to [the doc](k8s/dev-setup/development.md).

### 2. Running Locally
```bash
# Start the service
./bin/authz-service serve
# Or in development mode with debug logging
LOG_LEVEL=debug ./bin/authz-service serve
```
### 3. Testing
```bash
# Run unit tests
go test -v ./...
# Run specific package tests
go test -v ./internal/service/authz
# Run with coverage
go test -coverprofile=coverage.out ./...
go tool cover -html=coverage.out
# Run integration tests
go test -v ./tests/integration/...
```
### 4. Code Changes
When modifying code:
1. Update the implementation
2. Update or add tests
3. Run `make test` to verify
4. Update README.md if configuration changes
5. Commit with descriptive message
### 5. Adding New Services
To add a new integration:
1. Create package under `internal/integrations/`
2. Define client interface
3. Implement client with proper error handling
4. Add to `cmd/serve.go` initialization
5. Add configuration to config struct
6. Add tests
Example:
```go
// internal/integrations/myservice/client.go
package myservice
import "log/slog"
type Client interface {
    DoSomething() error
    Close() error
}
type MyClient struct {
    logger *slog.Logger
}
func NewClient(address string, logger *slog.Logger) (*MyClient, error) {
    // Implementation
    return &MyClient{logger: logger}, nil
}
func (c *MyClient) DoSomething() error {
    // Implementation
    return nil
}
func (c *MyClient) Close() error {
    return nil
}
```
### 6. Adding New gRPC Services
1. Update `api/proto/v1/*.proto`
2. Generate code: `buf generate api/proto`
3. Implement handler in service package
4. Register handler in server package
5. Add REST annotations for gateway
6. Test with grpcurl
## Testing Strategy

### Unit Tests

Located co-located with source code (Go best practice):

```
internal/service/permissions/
├── service.go
├── service_test.go
└── mocks/                   (auto-generated by go:generate)
    └── mock_valkey.go
```

Tests use [uber-go/mock](https://github.com/uber-go/mock) (GoMock) with `go:generate`:

```go
//go:generate mockgen -source=../../integrations/valkey/client.go -destination=mocks/mock_valkey.go -package=mocks CacheClient

package permissions

import "github.com/uber-go/mock/gomock"

func TestService(t *testing.T) {
    ctrl := gomock.NewController(t)
    defer ctrl.Finish()
    
    mockCache := mocks.NewMockCacheClient(ctrl)
    mockCache.EXPECT().
        Set(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
        Return(nil).
        Times(1)
    
    // Test code...
}
```

**Generate mocks:**
```bash
make mocks          # Regenerate all mocks
go generate ./...   # Alternative: regenerate everything
```

**Benefits:**
- ✅ No manual mock code to maintain
- ✅ Auto-generated from interfaces
- ✅ Fluent assertion API (`.EXPECT()`)
- ✅ Verify calls, order, counts, arguments
- ✅ Standard Uber Go pattern

### Integration Tests

Located in `tests/integration/`, use testcontainers:

### Mockable Interfaces

Integration packages define mockable interfaces:
- `valkey.CacheClient` - Cache operations
- `openfga.Client` - Authorization checks
- `sts.TokenClient` - Token validation

Services accept interfaces (not concrete types):

```go
type Service struct {
    cache  valkey.CacheClient
    logger *slog.Logger
}
```

This allows testable services without touching real external services.
## Project Structure Best Practices
### Packages
- `cmd/`: CLI commands only
- `internal/`: All internal code (not importable from outside)
  - `integrations/`: External service wrappers
  - `server/`: API servers (gRPC, REST)
  - `service/`: Business logic
  - `testutil/`: Testing helpers
- `api/proto/`: Protocol buffer definitions
- `tests/`: Test suites (unit, integration)
- `docker/`: Docker-related files
- `k8s/`: Kubernetes manifests
- `scripts/`: Build and utility scripts
### No `pkg/` Directory
Following Go best practices, we don't use `pkg/` directory. All code that shouldn't be imported goes in `internal/`.
### Naming Conventions
- **Packages**: lowercase, no underscores
- **Exported types**: PascalCase
- **Functions**: PascalCase (exported) or camelCase (unexported)
- **Constants**: ALL_CAPS (for exported package constants)
- **Variables**: camelCase
## Debugging
### Enable Debug Logging
```bash
LOG_LEVEL=debug ./bin/authz-service serve
LOG_FORMAT=text ./bin/authz-service serve
```
### Using Delve Debugger
```bash
go install github.com/go-delve/delve/cmd/dlv@latest
dlv debug ./cmd/server
(dlv) break main.main
(dlv) continue
(dlv) next
(dlv) print variable
```
### Checking Service Health
```bash
# gRPC health
grpcurl -plaintext localhost:9090 grpc.health.v1.Health/Check
# REST health
curl http://localhost:8888/healthz
# Container logs
docker logs <container-name>
```
## Performance Considerations
### Caching
- Authorization decisions cached in Valkey
- Cache key: `authz:{user}:{resource}:{action}`
- Cache TTL: Configurable per decision type
### Connection Pooling
- Valkey: Configurable pool size (default: 10)
- gRPC: Multiplexed over single connection
### Timeouts
All external service calls have configurable timeouts:
```go
ctx, cancel := context.WithTimeout(context.Background(), cfg.OpenFGA.Timeout)
defer cancel()
resp, err := client.Check(ctx, req)
```
## Common Tasks
### Adding a New Environment Variable
1. Edit `cmd/serve.go` - add field to config struct
2. Edit README.md - document in Configuration section
3. Edit `.env.example` - add example value
### Changing Default Port
1. Edit `cmd/serve.go` - change `default` tag
2. Edit README.md - update default value
3. Edit docker compose files - update port mappings
### Adding a New Integration
1. Create `internal/integrations/myservice/`
2. Implement client interface
3. Add config struct to `cmd/serve.go`
4. Initialize in `initializeIntegrations()`
5. Document in README.md
6. Add tests
### Running Specific Tests
```bash
# Single test function
go test -v -run TestPermissionRegister ./tests/unit
# Single package
go test -v ./internal/service/permissions
# With coverage
go test -v -cover ./...
```
## Contributing Guidelines
See [CONTRIBUTING.md](CONTRIBUTING.md) for code review and PR process.
## Useful Commands
```bash
# Download dependencies
go mod download
go mod tidy
# Format code
go fmt ./...
# Lint code
go vet ./...
golangci-lint run
# Build
go build -o bin/authz-service ./cmd/server
# Test
go test ./...
go test -v ./...
go test -short ./...
# Benchmark
go test -bench=. ./...
# Update dependencies
go get -u ./...
go mod tidy
```
## Troubleshooting Common Issues
### Port Already in Use
```bash
lsof -i :9090
kill -9 <PID>
```
### Import Cycle
Check for circular imports between packages. Split functionality if needed.
### Context Deadline Exceeded
Increase timeout in configuration:
```bash
export OPENFGA_TIMEOUT=30s
./bin/authz-service serve
```
### Configuration Not Loading
Check environment variables:
```bash
env | grep GRPC_PORT
LOG_LEVEL=debug ./bin/authz-service serve
```
