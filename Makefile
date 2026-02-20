.PHONY: help release-manifest build test clean deps start-deps stop-deps setup-protogen buf-update protogen protogen-client test-int coverage docker dev run mocks

BUF=./bin/buf
MAKEFLAGS += --no-print-directory

help:
	@echo "Authorization Service - Available targets:"
	@echo ""
	@echo "  make build        - Build the binary"
	@echo "  make test         - Run unit tests"
	@echo "  make test-int     - Run integration tests"
	@echo "  make coverage     - Generate coverage report"
	@echo "  make protogen     - Generate protobuf code"
	@echo "  make mocks        - Generate mocks using go:generate"
	@echo "  make deps         - Download Go dependencies"
	@echo "  make start-deps   - Start Docker dependencies"
	@echo "  make stop-deps    - Stop Docker dependencies"
	@echo "  make clean        - Clean build artifacts"
	@echo "  make docker       - Build Docker image"
	@echo "  make run          - Run service (requires dependencies)"
	@echo "  make dev          - Full dev setup (build + start deps + run)"
	@echo ""

release-manifest:
	@VERSION=$$(sed -n 's/.*Version = "\(.*\)".*/\1/p' cmd/root.go); \
	printf '{\n  ".": "%s"\n}\n' "$$VERSION" > .release-please-manifest.json

build: release-manifest
	@./scripts/build.sh

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

run: build
	@echo "Starting service..."
	@./bin/authz-service serve

dev: build start-deps
	@echo "Starting service in development mode..."
	@LOG_LEVEL=debug ./bin/authz-service serve

openapi-v3:
	cd openapi && go mod tidy && go run convert.go
.PHONY: openapi-v3


.DEFAULT_GOAL := help

