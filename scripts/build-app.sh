#!/bin/bash

set -e

PURPLE='\033[0;35m'
GREEN='\033[0;32m'
BLUE='\033[0;34m'
NC='\033[0m'

APP_BINARY_PATH="${APP_BINARY_PATH:-bin/app}"
CGO_ENABLED="${CGO_ENABLED:-0}"
GOOS="${GOOS:-$(go env GOOS)}"
GOARCH="${GOARCH:-$(go env GOARCH)}"

echo -e "${PURPLE}🔨 Building Authorization Service (${GOOS}/${GOARCH})...${NC}"

echo ""
echo -e "${BLUE}🏗️ Building binary (${APP_BINARY_PATH})...${NC}"
CGO_ENABLED="${CGO_ENABLED}" GOOS="${GOOS}" GOARCH="${GOARCH}" go build \
    -a -buildvcs \
    -ldflags "-s -w" \
    -o "${APP_BINARY_PATH}"

echo -e "${BLUE}📊 Binary size: $(du -h "${APP_BINARY_PATH}" | cut -f1)${NC}"

echo -e "${GREEN}✓ Build complete!${NC}"
echo ""
echo "Run the service with:"
echo "  ./${APP_BINARY_PATH}"
