#!/bin/bash

set -e

SCRIPT_DIR="$( cd "$( dirname "${BASH_SOURCE[0]}" )" && pwd )"
PROJECT_ROOT="$( cd "$SCRIPT_DIR/.." && pwd )"

# Colors
GREEN='\033[0;32m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

echo -e "${BLUE}🗑️  Stopping all containers...${NC}"

# Stop dependencies
cd "$PROJECT_ROOT/docker/dependencies"
docker compose down 2>/dev/null || true

# Remove network
docker network rm authz-network 2>/dev/null || true

echo -e "${GREEN}✓ All containers stopped${NC}"
