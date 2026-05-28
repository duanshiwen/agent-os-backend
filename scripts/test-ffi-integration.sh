#!/usr/bin/env bash
set -euo pipefail

SDK_DIR="${SDK_DIR:-/Users/yakii/code/agent-os/Infrastructure/connor-agent-core}"
ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
LIB_PATH="${AGENTOS_FFI_LIBRARY_PATH:-$SDK_DIR/target/release/libagentos_ffi.dylib}"

cd "$SDK_DIR"
cargo build -p agentos-ffi --release

cd "$ROOT_DIR"
AGENTOS_FFI_LIBRARY_PATH="$LIB_PATH" go test ./internal/service -run TestFFIVerifierIntegration -count=1 -v
