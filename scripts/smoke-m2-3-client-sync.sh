#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT_DIR"

echo "==> M2.3 client sync consumer contract: router coverage"
go test ./internal/handler -run 'TestM23ClientKnowledgeSyncConsumerFlow' -count=1

echo
echo "M2.3 client sync consumer smoke passed."
