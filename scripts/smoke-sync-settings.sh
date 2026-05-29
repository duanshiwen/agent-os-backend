#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT_DIR"

echo "==> M2.1 sync settings smoke: service coverage"
go test ./internal/service -run 'Test(SkillSettings|AgentSettings|ServerConnections|SyncContract)' -count=1

echo
echo "==> M2.1 sync settings smoke: router coverage"
go test ./internal/handler -run 'Test(SkillSettingsEnableEmitsPullableSyncEvent|AgentSettingsUpdateEmitsPullableSyncEvent|ServerConnectionsAddEmitsPullableSyncEvent)' -count=1

echo
echo "M2.1 sync settings smoke passed."
