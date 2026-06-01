#!/usr/bin/env bash
set -euo pipefail

BASE_URL="${BASE_URL:-http://localhost:8080}"
RUN_LIVE_SMOKES="${RUN_LIVE_SMOKES:-0}"
RUN_LOCAL_HTTP_SEMANTIC="${RUN_LOCAL_HTTP_SEMANTIC:-0}"
RUN_MIGRATION_GATE="${RUN_MIGRATION_GATE:-0}"
EMBEDDING_ENDPOINT="${EMBEDDING_ENDPOINT:-http://localhost:8091}"

require_tool() {
  if ! command -v "$1" >/dev/null 2>&1; then
    echo "missing required tool: $1" >&2
    exit 1
  fi
}

require_tool go
require_tool bash
require_tool python3

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$repo_root"

echo "==> Go tests"
go test ./...

echo "==> Shell syntax checks"
for script in scripts/*.sh; do
  bash -n "$script"
done

echo "==> Python syntax checks"
python3 -m py_compile examples/sage-plugins/hotel-booking/mock_server.py

echo "==> FFI bundled artifact checks"
./scripts/check-ffi-artifacts.sh

echo "==> Stage 5A client-ready sync bridge smoke"
./scripts/smoke-stage5a-sync-bridge.sh

echo "==> SAGE client runtime contract gate"
./scripts/check-sage-client-runtime-contract.sh

echo "==> Governance client error contract gate"
./scripts/check-governance-client-error-contract.sh

if [[ "$RUN_MIGRATION_GATE" == "1" ]]; then
  echo "==> PostgreSQL migration apply gate"
  ./scripts/check-migrations-local.sh
else
  echo "==> Skipping PostgreSQL migration apply gate (set RUN_MIGRATION_GATE=1)"
fi

if [[ "$RUN_LIVE_SMOKES" == "1" ]]; then
  require_tool curl
  require_tool jq
  require_tool openssl
  require_tool xxd
  echo "==> Health check ${BASE_URL}"
  curl -fsS "${BASE_URL}/health" | jq '{status, checks}'
  echo "==> Object storage smoke"
  BASE_URL="$BASE_URL" ./scripts/smoke-object-storage.sh
  echo "==> SAGE runtime/control-plane smoke"
  BASE_URL="$BASE_URL" ./scripts/smoke-sage-plugin-runtime.sh
  echo "==> Governance enforcement smoke"
  BASE_URL="$BASE_URL" ./scripts/smoke-governance-enforcement.sh
  echo "==> Governance enforce-readiness smoke"
  BASE_URL="$BASE_URL" ./scripts/smoke-governance-enforce-readiness.sh
else
  echo "==> Skipping live smokes (set RUN_LIVE_SMOKES=1 with a running local stack)"
fi

if [[ "$RUN_LOCAL_HTTP_SEMANTIC" == "1" ]]; then
  require_tool curl
  require_tool jq
  echo "==> Real local_http semantic smoke"
  BASE_URL="$BASE_URL" EMBEDDING_ENDPOINT="$EMBEDDING_ENDPOINT" ./scripts/smoke-kb-semantic-local-http.sh
else
  echo "==> Skipping real local_http semantic smoke (set RUN_LOCAL_HTTP_SEMANTIC=1)"
fi

printf '\nRelease gate local checks passed.\n'
