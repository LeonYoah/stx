#!/bin/bash
# Copyright 2024 Apache SeaTunnel
# Licensed under the Apache License, Version 2.0

# Script to generate Go code from Protocol Buffer definitions

set -e

PROTO_DIR="internal/proto"
AGENT_PROTO_DIR="${PROTO_DIR}/agent"

echo "Generating Go code from Protocol Buffers..."

# Check if protoc is installed
if ! command -v protoc &> /dev/null; then
    echo "Error: protoc is not installed. Please install Protocol Buffers compiler."
    echo "  - macOS: brew install protobuf"
    echo "  - Linux: apt-get install protobuf-compiler"
    echo "  - Windows: choco install protoc"
    exit 1
fi

export PATH="${PATH}:$(go env GOPATH)/bin"

# Check if protoc-gen-go is installed
if ! command -v protoc-gen-go &> /dev/null; then
    echo "Installing protoc-gen-go..."
    go install google.golang.org/protobuf/cmd/protoc-gen-go@latest
fi

# Check if protoc-gen-go-grpc is installed
if ! command -v protoc-gen-go-grpc &> /dev/null; then
    echo "Installing protoc-gen-go-grpc..."
    go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@latest
fi

# Generate Go code for internal/proto/agent
echo "Generating internal/proto/agent proto..."
protoc \
    --proto_path=. \
    --go_out=. \
    --go_opt=paths=source_relative \
    --go-grpc_out=. \
    --go-grpc_opt=paths=source_relative \
    "${AGENT_PROTO_DIR}/agent.proto"

# Generate Go code for agent module
echo "Generating agent module proto..."
protoc \
    --proto_path="${PROTO_DIR}" \
    --go_out=. \
    --go_opt=paths=source_relative \
    --go-grpc_out=. \
    --go-grpc_opt=paths=source_relative \
    "${AGENT_PROTO_DIR}/agent.proto"

echo "Proto generation completed successfully!"
