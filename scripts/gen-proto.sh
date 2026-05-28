#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
SDK_DIR="${SDK_DIR:-/Users/yakii/code/agent-os/Infrastructure/connor-agent-core}"
PROTO_DIR="$SDK_DIR/schemas/sidecar/v1"

export PATH="$(go env GOPATH)/bin:$PATH"

if ! command -v protoc >/dev/null 2>&1; then
  echo "protoc is required. On macOS: brew install protobuf" >&2
  exit 1
fi
if ! command -v protoc-gen-go >/dev/null 2>&1; then
  echo "protoc-gen-go is required. Run: go install google.golang.org/protobuf/cmd/protoc-gen-go@latest" >&2
  exit 1
fi
if ! command -v protoc-gen-go-grpc >/dev/null 2>&1; then
  echo "protoc-gen-go-grpc is required. Run: go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@latest" >&2
  exit 1
fi

protoc \
  -I "$PROTO_DIR" \
  --go_out="$ROOT_DIR" --go_opt=module=github.com/agent-os/backend \
  --go-grpc_out="$ROOT_DIR" --go-grpc_opt=module=github.com/agent-os/backend \
  "$PROTO_DIR/sidecar.proto"

echo "generated sidecar protobuf stubs"
