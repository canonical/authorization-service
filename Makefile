# Variables
GO?=go
GO_BIN?=app
BINARY_PATH?=bin/$(GO_BIN)
BINARY_ARCHS?=amd64 arm64 riscv64
CMD_PATH=./
PROTO_PATH=api/proto/v1
CGO_ENABLED?=0
GOOS?=$(shell $(GO) env GOOS)
GOARCH?=$(shell $(GO) env GOARCH)
GO_TEST_PARALLEL?=10
BUF=./bin/buf

MAKEFLAGS += --no-print-directory
# Every target is PHONY
MAKEFLAGS += --always-make

help:
	@echo "Authorization Service - Available targets:"
	@echo ""
	@echo "  make build        - Build the binary"
	@echo "  make build-all    - Build binaries for all supported architectures (amd64, arm64, riscv64)"
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
	@echo "  make apply        - Deploy Authorization-service and dependencies using Terraform on K8s"
	@echo ""

release-manifest:
	@VERSION=$$(sed -n 's/.*Version = "\(.*\)".*/\1/p' internal/version/const.go); \
	printf '{\n  ".": "%s"\n}\n' "$$VERSION" > .release-please-manifest.json

# Build the binary
build: release-manifest
	@CGO_ENABLED=$(CGO_ENABLED) GOOS=$(GOOS) GOARCH=$(GOARCH) APP_BINARY_PATH=$(BINARY_PATH) ./scripts/build-app.sh

BUILD_ARCH_TARGETS := $(addprefix build-,$(BINARY_ARCHS))

# Build binaries for all supported architectures (amd64, arm64, riscv64)
build-all: $(BUILD_ARCH_TARGETS)

$(BUILD_ARCH_TARGETS): build-%: release-manifest
	@CGO_ENABLED=$(CGO_ENABLED) GOOS=linux GOARCH=$* APP_BINARY_PATH=bin/$(GO_BIN)-linux-$* ./scripts/build-app.sh

mocks:
	@command -v mockgen > /dev/null || ( \
    		echo "Installing mockgen..." && \
    		go install go.uber.org/mock/mockgen@v0.6.0 \
    	)
	@echo "Generating mocks..."
	@go generate ./...

test: mocks
	@./scripts/test.sh

test-int:
	@echo "Running integration tests..."
	@go test -v -race -tags integration ./tests/integration/... -timeout 5m

test-clear-cache:
	@echo "Clearing test cache..."
	@go clean -testcache

test-e2e:
	@echo "Not implemented yet"

coverage:
	@echo "Generating coverage report..."
	@go test -coverprofile=coverage.out ./...
	@go tool cover -html=coverage.out -o coverage.html
	@echo "Coverage report: coverage.html"


setup-protogen:
	@./scripts/setup-protogen.sh

buf-update: setup-protogen
	@echo "Updating Buf dependencies..."
	@$(BUF) dep update

protogen: buf-update
	@echo "Generating protobuf server code..."
	@$(BUF) generate api/proto

protogen-client: buf-update
	@echo "Generating protobuf client code..."
	@$(BUF) generate --template ./buf.gen.client.yaml

deps:
	@echo "Downloading Go dependencies..."
	@go mod download
	@go mod tidy

vendor: mocks
	$(GO) mod vendor
.PHONY: vendor

govulncheck: vendor
	$(GO) install golang.org/x/vuln/cmd/govulncheck@latest
	PATH="$$($(GO) env GOPATH)/bin:$$PATH" govulncheck ./...
.PHONY: govulncheck

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

dev: build start-deps
	@echo "Starting service in development mode..."
	@LOG_LEVEL=debug ./$(BINARY_PATH) serve --dev

openapi-v3:
	cd openapi && go mod tidy && go run convert.go

# Lint code (requires golangci-lint)
lint:
	golangci-lint run

# Deploy Authorization-service and dependencies using Terraform
apply:
	@if [ ! -d "terraform/.terraform" ]; then \
		echo "The Terraform workspace has not been initialised yet."; \
		echo "Initialising it now..."; \
		cd terraform && terraform init; \
	fi
	@echo "Applying Terraform configurations..."
	@cd terraform && terraform apply -auto-approve

.DEFAULT_GOAL := help
