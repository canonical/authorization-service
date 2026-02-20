.PHONY: help release-manifest build test clean deps start-deps stop-deps setup-protogen buf-update protogen protogen-client test-int coverage docker dev run mocks

.PHONY: all build test clean run install-deps proto

# Variables
GO_BIN?=app
BINARY_PATH=bin/$(GO_BIN)
CMD_PATH=./
PROTO_PATH=api/proto/v1
GOFLAGS?=-ldflags=-w -ldflags=-s -a -buildvcs
CGO_ENABLED?=0
GO?=go
GO_TEST_PARALLEL?=10

release-manifest:
	@VERSION=$$(sed -n 's/.*Version = "\(.*\)".*/\1/p' version/const.go); \
	printf '{\n  ".": "%s"\n}\n' "$$VERSION" > .release-please-manifest.json

# Build the binary
build:
	@echo "Building $(GO_BIN)..."
	@mkdir -p bin
	$(GO) build -o $(BINARY_PATH) $(CMD_PATH)
	@echo "Build complete: $(CMD_PATH)"
.PHONY: build

mocks:
	@command -v mockgen > /dev/null || ( \
    		echo "Installing mockgen..." && \
    		go install go.uber.org/mock/mockgen@v0.6.0 \
    	)
	@echo "Generating mocks..."
	@go generate ./internal/service/...

test: mocks
	@./scripts/test.sh

test-int:
	@echo "Running integration tests..."
	@go test -v ./tests/integration/... -timeout 5m

test-e2e:
	@echo "Not implemented yet"

coverage:
	@echo "Generating coverage report..."
	@go test -coverprofile=coverage.out ./...
	@go tool cover -html=coverage.out -o coverage.html
	@echo "Coverage report: coverage.html"

setup-protogen:
	@./scripts/setup-protogen.sh

buf-update:
	@echo "Updating Buf dependencies..."
	@$(BUF) dep update

protogen: buf-update
	@echo "Generating protobuf server code..."
	@$(BUF) generate api/proto

protogen-client: buf-update
	@echo "Generating protobuf client code..."
	@$(BUF) generate client/proto --template ./buf.gen.client.yaml

deps:
	@echo "Downloading dependencies..."
	@go mod download
	@go mod tidy

start-deps:
	@./scripts/start-deps.sh

stop-deps:
	@./scripts/stop-deps.sh

clean:
	@echo "Cleaning build artifacts..."
	@rm -rf bin/
	@rm -f coverage.out coverage.html
	@go clean

docker:
	@echo "Building Docker image..."
	@docker build -f docker/app/Dockerfile -t authorization-service:latest .

# Run the service
run: build
	@echo "Starting $(BINARY_NAME)..."
	./$(BINARY_PATH)

# Clean build artifacts
clean:
	@echo "Cleaning..."
	rm -rf bin/
	rm -f coverage.out coverage.html
	rm -rf keys/
	@echo "Clean complete"

# Generate protobuf code (requires protoc)
proto:
	@echo "Generating protobuf code..."
	protoc --go_out=. --go_opt=paths=source_relative \
		--go-grpc_out=. --go-grpc_opt=paths=source_relative \
		$(PROTO_PATH)/sts.proto
	@echo "Protobuf generation complete"

# Format code
fmt:
	go fmt ./...

# Lint code (requires golangci-lint)
lint:
	golangci-lint run

