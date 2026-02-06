#!/bin/bash

set -e

PURPLE='\033[0;35m'
GREEN='\033[0;32m'
BLUE='\033[0;34m'
NC='\033[0m'

echo -e "${PURPLE}🔨 Building Authorization Service...${NC}"
echo -e "${BLUE}📦 Generating Protocol Buffers...${NC}"
make protogen

echo ""
echo -e "${BLUE}🏗️ Building binary...${NC}"
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build \
    -ldflags "-X main.Version=$(git describe --tags --always 2>/dev/null || echo 'dev')" \
    -o bin/authz-service \
    ./cmd

echo -e "${BLUE}📊 Binary size: $(du -h bin/authz-service | cut -f1)${NC}"

echo -e "${GREEN}✓ Build complete!${NC}"
echo ""
echo "Run the service with:"
echo "  ./bin/authz-service"
