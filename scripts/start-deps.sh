#!/bin/bash

set -e

SCRIPT_DIR="$( cd "$( dirname "${BASH_SOURCE[0]}" )" && pwd )"
PROJECT_ROOT="$( cd "$SCRIPT_DIR/.." && pwd )"

# Colors
GREEN='\033[0;32m'
BLUE='\033[0;34m'
YELLOW='\033[1;33m'
RED='\033[0;31m'
NC='\033[0m' # No Color

echo -e "${BLUE}🐳 Starting dependencies...${NC}"

cd "$PROJECT_ROOT/docker/dependencies"

echo -e "${BLUE}Creating network...${NC}"
docker network create authz-network 2>/dev/null || true

echo -e "${BLUE}Starting services...${NC}"
docker-compose up -d

echo ""
echo -e "${YELLOW}⏳ Waiting for services to be healthy...${NC}"

# Wait for OpenFGA
echo -n "Waiting for OpenFGA..."
for i in {1..30}; do
    if curl -s http://localhost:8080/healthz > /dev/null 2>&1; then
        echo -e " ${GREEN}✓${NC}"
        break
    fi
    echo -n "."
    sleep 1
    if [ $i -eq 30 ]; then
        echo -e " ${RED}✗ Timeout${NC}"
        exit 1
    fi
done

# Wait for Valkey
echo -n "Waiting for Valkey..."
for i in {1..30}; do
    if docker exec authz-valkey valkey-cli ping > /dev/null 2>&1; then
        echo -e " ${GREEN}✓${NC}"
        break
    fi
    echo -n "."
    sleep 1
    if [ $i -eq 30 ]; then
        echo -e " ${RED}✗ Timeout${NC}"
        exit 1
    fi
done

echo ""
echo -e "${GREEN}✓ All dependencies are running!${NC}"
echo ""
echo "Service endpoints:"
echo "  OpenFGA HTTP:  http://localhost:8080"
echo "  OpenFGA gRPC:  localhost:8081"
echo "  OpenFGA UI:    http://localhost:3000"
echo "  Valkey:        localhost:6379"
echo ""
echo "To stop dependencies:"
echo "  cd docker/dependencies && docker-compose down"
