#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT_DIR"

echo "==> Governance client error contract gate"
go test ./internal/handler -run 'TestGovernanceHandlerClientErrorContract$' -count=1 -v

printf '\nGovernance client error contract gate passed.\n'
