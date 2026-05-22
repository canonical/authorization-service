#!/bin/bash
# Copyright 2025 Canonical Ltd.
# SPDX-License-Identifier: AGPL-3.0-only

set -e

DIR="$( dirname -- "$0"; )"
BIN="$DIR/../bin"
VERSION="v1.50.0" # last release as of 2026-02-03

if [ "$1" = "--latest" ]; then
  VERSION=$(curl -s "https://api.github.com/repos/bufbuild/buf/releases/latest" | jq -r '.name')
fi

echo "Attempting to install buf version $VERSION"

# Create bin directory if it doesn't exist
mkdir -p "${BIN}"

# Download buf binary
curl -sSL "https://github.com/bufbuild/buf/releases/download/${VERSION}/buf-$(uname -s)-$(uname -m)" -o "${BIN}/buf"
chmod +x "${BIN}/buf"

# Verify buf was downloaded successfully
if [ ! -f "${BIN}/buf" ]; then
  echo "ERROR: Failed to download buf binary"
  exit 1
fi

echo "Attempting to install necessary protoc plugins via 'go install'"
echo "Make sure your GOPATH/bin is in your PATH environment variable"

# Plugin Go base
go install google.golang.org/protobuf/cmd/protoc-gen-go@latest

# Plugin gRPC
go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@latest

# Plugin grpc-gateway
go install github.com/grpc-ecosystem/grpc-gateway/v2/protoc-gen-grpc-gateway@v2.27.1

# Plugin OpenAPI v2
go install github.com/grpc-ecosystem/grpc-gateway/v2/protoc-gen-openapiv2@v2.26.1

echo "Installation succeeded"
