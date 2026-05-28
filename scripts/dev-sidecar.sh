#!/usr/bin/env bash
set -euo pipefail

SDK_DIR="${SDK_DIR:-/Users/yakii/code/agent-os/Infrastructure/connor-agent-core}"
SOCKET="${RUST_SIDECAR_SOCKET:-/tmp/agentos-sidecar.sock}"

cd "$SDK_DIR"
exec cargo run -p agentos-sidecar -- --socket "$SOCKET"
