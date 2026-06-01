#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT_DIR"

echo "==> SAGE client runtime contract gate"
go test ./internal/service -run 'TestSAGEPluginService(ClientRuntimeContract|GovernanceEnforceClientRuntimeOutcomes)$' -count=1 -v

printf '\nSAGE client runtime contract gate passed.\n'
