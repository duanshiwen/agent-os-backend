#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT_DIR"

echo "==> M2.2 aggregate smoke: knowledge sync"
./scripts/smoke-knowledge-sync.sh

echo
echo "==> M2.2 aggregate smoke: sync contract tests"
go test ./internal/service -run 'TestSyncContract' -count=1

echo
echo "==> M2.2 aggregate smoke: full backend test suite"
go test ./...

echo
echo "M2.2 aggregate smoke passed."
