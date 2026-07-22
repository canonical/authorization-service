#!/bin/bash

set -e

# Colors
GREEN='\033[0;32m'
BLUE='\033[0;34m'
RED='\033[0;31m'
NC='\033[0m' # No Color

echo -e "${BLUE}🧪 Running tests...${NC}"

# Run unit tests
echo -e "${BLUE}📝 Running unit tests (short)...${NC}"
go test -v -short -race ./... -coverprofile=coverage.out

# Display coverage
COVERAGE=$(go tool cover -func=coverage.out | grep total | awk '{print $3}')
echo -e "${GREEN}✓ Total coverage: ${COVERAGE}${NC}"

# Generate HTML coverage report
go tool cover -html=coverage.out -o coverage.html
echo -e "${GREEN}✓ Coverage report generated: coverage.html${NC}"

echo ""
echo "To run integration tests (requires Docker):"
echo "  go test -v -race -tags integration ./tests/integration/... -timeout 5m"
