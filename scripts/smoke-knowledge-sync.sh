#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT_DIR"

echo "==> M2.2 knowledge sync smoke: service coverage"
go test ./internal/service -run 'TestKnowledgeEntries' -count=1

echo
echo "==> M2.2 knowledge sync smoke: router coverage"
go test ./internal/handler -run 'TestKnowledgeEntriesMutationsEmitPullableSyncEvents' -count=1

echo
echo "M2.2 knowledge sync smoke passed."
