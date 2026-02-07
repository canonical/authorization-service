# Makefile for Authorization Service (Cerberus)

.PHONY: all build test clean run install-deps proto

# Variables
BINARY_NAME=app
BINARY_PATH=bin/$(BINARY_NAME)
CMD_PATH=./cmd/authz
PROTO_PATH=api/proto/v1
GOFLAGS?=-ldflags=-w -ldflags=-s -a -buildvcs
CGO_ENABLED?=0

# Default target
all: build

# Install Go dependencies
install-deps:
	go mod download
	go mod tidy

# Build the binary
build:
	@echo "Building $(BINARY_NAME)..."
	@mkdir -p bin
	go build -o $(BINARY_PATH) $(CMD_PATH)
	@echo "Build complete: $(BINARY_PATH)"

# Run tests
test:
	@echo "Running tests..."
	go test ./... -v -cover

# Run tests with coverage
test-coverage:
	@echo "Running tests with coverage..."
	go test ./... -coverprofile=coverage.out
	go tool cover -html=coverage.out -o coverage.html
	@echo "Coverage report generated: coverage.html"

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

