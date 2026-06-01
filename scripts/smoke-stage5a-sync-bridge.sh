#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT_DIR"

echo "==> Stage 5A client-ready sync bridge FFI smoke"
go test ./internal/service -run 'TestFFIClientReadySyncBridgeIntegration' -count=1 -v
