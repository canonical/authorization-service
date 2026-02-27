# Contributing to Authorization Service
Thank you for your interest in contributing to the Authorization Service! This document provides guidelines and instructions for contributing.
## Code of Conduct
Be respectful, inclusive, and professional in all interactions. We're building a collaborative environment for everyone.
## Getting Started
### Prerequisites
- **Go**: 1.25 or later
- **Git**: For version control
- **Docker**: For local testing
- **Make**: For build automation
### Setup Development Environment
```bash
# Clone repository
git clone https://github.com/canonical/authorization-service.git
cd authorization-service
# Install dependencies
go mod download
# Build the project
make build
# Start dependencies
make start-deps
# Run tests
make test
```
## Development Workflow
### 1. Create a Feature Branch
```bash
git checkout -b feature/your-feature-name
# or for bugfixes
git checkout -b fix/issue-number-description
```
### 2. Make Your Changes
Follow these guidelines:
- **Code Style**: Use `go vet` (don't use `go fmt` as it uses tabs instead of spaces)
- **Naming**: Follow Go naming conventions
  - Packages: `lowercase`
  - Types: `PascalCase`
  - Functions: `PascalCase` (exported) or `camelCase` (unexported)
  - Constants: `ALL_CAPS`
- **Error Handling**: Always handle errors explicitly
- **Logging**: Use structured logging with `log/slog`
- **Comments**: Comment exported types and functions
### 3. Write Tests
All code changes should include tests:
```bash
# Unit tests for new functionality
# Located in tests/unit/
# Integration tests for external service interactions
# Located in tests/integration/
# Run tests
make test
# Run with coverage
make coverage
```
### 4. Update Documentation
Update these files when making changes:
- **README.md**: Configuration changes, new features
- **DEVELOPMENT.md**: Architecture changes, new patterns
- **Any .proto files**: Update API docs
- **Code comments**: For complex logic
### 5. Commit Changes
Use descriptive commit messages following conventional commits:
```bash
git add .
git commit -m "feat: add new authorization check method
- Implement check method in authz service
- Add unit tests for check logic
- Update configuration for timeout
Fixes #123"
```
Commit types:
- `feat`: New feature
- `fix`: Bug fix
- `docs`: Documentation changes
- `test`: Test additions/changes
- `refactor`: Code refactoring
- `perf`: Performance improvements
- `chore`: Build, dependency updates
- `ci`: CI/CD changes
### 6. Push and Create Pull Request
```bash
git push origin feature/your-feature-name
```
Then create a Pull Request on GitHub with:
- **Title**: Clear and descriptive
- **Description**: Explain what and why
- **Linked Issues**: Reference any related issues
- **Tests**: Verify all tests pass
- **Documentation**: Updated as needed
## Code Review Process
### What Reviewers Look For
1. **Correctness**: Does the code work as intended?
2. **Design**: Does it fit the architecture?
3. **Tests**: Are there adequate tests?
4. **Documentation**: Is it well-documented?
5. **Performance**: Are there any performance concerns?
6. **Security**: Are there security implications?
### Review Feedback
- Be respectful and constructive
- Ask questions to understand intent
- Suggest improvements
- Reference relevant docs or examples
### Addressing Review Comments
1. Discuss if you disagree (respectfully)
2. Make requested changes
3. Push updated code
4. Request re-review
## Code Standards
### Go Style
```go
// Good
func (s *Service) CheckAuthorization(ctx context.Context, req *CheckRequest) (*CheckResponse, error) {
    if req == nil {
        return nil, fmt.Errorf("request cannot be nil")
    }
    s.logger.Info("Checking authorization", "user", req.User, "resource", req.Resource)
    // Implementation
    return &CheckResponse{Allowed: true}, nil
}
// Bad
func (s *Service) CheckAuth(ctx context.Context, r *CheckRequest) (*CheckResponse, error) {
    // Implementation without comments or logging
}
```
### Error Handling
```go
// Good
if err != nil {
    return nil, fmt.Errorf("failed to check authorization: %w", err)
}
// Bad
if err != nil {
    panic(err)  // Don't panic
}
if err != nil {
    log.Fatal(err)  // Don't use Fatal
}
```
### Logging
```go
// Good - structured logging
logger.Info("Cache hit", "key", cacheKey, "ttl_remaining", ttl)
logger.Error("Failed to connect", "error", err, "service", "OpenFGA")
// Bad
fmt.Println("Error:", err)
log.Printf("Something happened")
```

### Testing

Unit tests are co-located with source code (same package) and use [uber-go/mock](https://github.com/uber-go/mock) with `go:generate`.

**File location:** `internal/service/{package}/service_test.go`

```go
//go:generate mockgen -source=../../integrations/valkey/client.go -destination=mocks/mock_valkey.go -package=mocks CacheClient

package myservice

import "github.com/canonical/authorization-service/internal/service/myservice/mocks"

func TestServiceCheck(t *testing.T) {
    ctrl := gomock.NewController(t)
    defer ctrl.Finish()
    
    mockCache := mocks.NewMockCacheClient(ctrl)
    mockCache.EXPECT().Get(gomock.Any(), gomock.Any()).Return("", gomock.Any()).Times(1)
    
    svc := NewService(mockCache, logger)
    resp, err := svc.Check(context.Background(), &CheckRequest{
        User: "alice", Resource: "doc1", Action: "read",
    })
    
    if err != nil {
        t.Fatalf("unexpected error: %v", err)
    }
    if !resp.Allowed {
        t.Error("expected authorization to be allowed")
    }
}
```

**Running tests:**
```bash
make mocks  # Regenerate mocks via go:generate
make test   # Run tests (auto-regenerates mocks)
```

## Adding New Features
### New Integration Service
1. Create package under `internal/integrations/`
2. Define client interface
3. Implement client
4. Add configuration to `cmd/serve.go`
5. Initialize in `initializeIntegrations()`
6. Document in README.md
7. Add tests
### New gRPC Service
1. Update `api/proto/v1/service.proto`
2. Add service methods
3. Run `buf generate api/proto`
4. Implement service handler
5. Register in gRPC server
6. Add REST annotations (if applicable)
7. Document API endpoints
8. Add tests
### Configuration Changes
1. Add field to config struct in `cmd/serve.go`
2. Use envconfig tags: `envconfig:"VAR_NAME" default:"value"`
3. Update README.md Configuration section
4. Update `.env.example`
5. Update DEVELOPMENT.md if architectural change
6. Add tests if validation logic added
## Pull Request Checklist
Before submitting a PR, verify:
- [ ] Code follows Go style guide (`go fmt`, `go vet`)
- [ ] All tests pass (`make test`)
- [ ] New tests added for new functionality
- [ ] No test coverage decreased (`make coverage`)
- [ ] Code compiles without warnings
- [ ] README.md updated if needed
- [ ] DEVELOPMENT.md updated if needed
- [ ] Documentation comments added
- [ ] No unrelated changes included
- [ ] Commit messages are descriptive
- [ ] No API breaking changes (or documented)
## Types of Contributions
### Bug Reports
Include:
- What you expected to happen
- What actually happened
- Steps to reproduce
- Go version and OS
- Relevant logs or error messages
### Feature Requests
Include:
- Use case and problem statement
- Proposed solution
- Alternative approaches considered
- Any examples or references
### Documentation
- Fix typos and unclear sections
- Add examples
- Improve diagrams
- Expand troubleshooting section
- Clarify complex concepts
### Code
- New features
- Bug fixes
- Performance improvements
- Test coverage
- Code refactoring
## Dependency Management
### Adding Dependencies
Use Go modules:
```bash
go get github.com/user/package@version
go mod tidy
```
In PR, justify why the dependency is needed:
- Does it avoid reimplementing complex logic?
- Is it well-maintained?
- What's the license?
- Any security concerns?
### Updating Dependencies
```bash
go get -u ./...
go mod tidy
# Run tests to verify compatibility
make test
```
## Documentation Standards
### Code Comments
```go
// CheckAuthorization performs an authorization check.
// It returns true if the user is allowed to perform the action on the resource.
func (s *Service) CheckAuthorization(ctx context.Context, req *CheckRequest) (*CheckResponse, error) {
    // Implementation
}
```
### README Sections
- **Overview**: What is this for?
- **Prerequisites**: What's needed?
- **Installation**: How to set up?
- **Configuration**: Environment variables
- **Usage**: How to use it?
- **API**: Available endpoints
- **Troubleshooting**: Common issues
### Examples
Include runnable examples for:
- Configuration setup
- API usage
- Common tasks
- Debugging steps
## Project Structure
Maintain these conventions:
```
cmd/           # CLI commands
internal/      # All internal packages (not importable)
api/proto/     # Protocol buffer definitions
tests/         # Test suites
docker/        # Docker files
k8s/           # Kubernetes files
scripts/       # Build scripts
```
No `pkg/` directory. Use `internal/` for everything not meant to be exported.
## Performance Considerations
When contributing:
- Avoid blocking operations in hot paths
- Use connection pooling
- Implement proper timeouts
- Consider memory usage
- Profile before/after for improvements
- Document performance tradeoffs
## Security Considerations
When contributing:
- No hardcoded credentials
- Validate all inputs
- Handle errors without leaking info
- Use proper TLS where applicable
- Log without sensitive data
- Keep dependencies updated
- Report security issues privately
## Questions or Need Help?
- Check [DEVELOPMENT.md](DEVELOPMENT.md) for architecture details
- Review existing code for patterns
- Open an issue for clarification
- Ask in PR comments
## License
By contributing, you agree that your contributions will be licensed under the project's license (see [LICENSE](LICENSE)).
Thank you for contributing!
