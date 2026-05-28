#!/usr/bin/env bash
set -euo pipefail

SDK_DIR="${SDK_DIR:-/Users/yakii/code/agent-os/Infrastructure/connor-agent-core}"
ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
SOCKET="${AGENTOS_SIDECAR_INTEGRATION_SOCKET:-/tmp/agentos-sidecar-integration.sock}"
LOG_FILE="${AGENTOS_SIDECAR_INTEGRATION_LOG:-/tmp/agentos-sidecar-integration.log}"

rm -f "$SOCKET"
cd "$SDK_DIR"
cargo run -p agentos-sidecar -- --socket "$SOCKET" > "$LOG_FILE" 2>&1 &
SIDECAR_PID=$!

cleanup() {
  kill "$SIDECAR_PID" >/dev/null 2>&1 || true
  rm -f "$SOCKET"
}
trap cleanup EXIT

for _ in $(seq 1 80); do
  if [ -S "$SOCKET" ]; then
    break
  fi
  sleep 0.25
done

if [ ! -S "$SOCKET" ]; then
  echo "sidecar did not create socket at $SOCKET" >&2
  cat "$LOG_FILE" >&2 || true
  exit 1
fi

cd "$ROOT_DIR"
AGENTOS_SIDECAR_INTEGRATION_SOCKET="$SOCKET" go test ./internal/sidecar -run TestClientIntegrationWithRustSidecar -count=1 -v
