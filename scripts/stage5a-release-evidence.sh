#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT_DIR"

OUTPUT_PATH="${OUTPUT_PATH:-tmp/stage5a-release-evidence.md}"
RUN_SDK_TESTS="${RUN_SDK_TESTS:-0}"
SDK_ROOT="${SDK_ROOT:-/Users/yakii/code/agent-os/Infrastructure/connor-agent-core}"

require_tool() {
  if ! command -v "$1" >/dev/null 2>&1; then
    echo "missing required tool: $1" >&2
    exit 1
  fi
}

require_tool git
require_tool go
require_tool bash
require_tool python3

if [[ "$RUN_SDK_TESTS" == "1" ]]; then
  require_tool cargo
fi

mkdir -p "$(dirname "$OUTPUT_PATH")" tmp/stage5a-evidence-logs

branch="$(git branch --show-current)"
commit="$(git rev-parse --short HEAD)"
full_commit="$(git rev-parse HEAD)"
started_at="$(date -u +%Y-%m-%dT%H:%M:%SZ)"

RESULT_ROWS=()
LOG_FILES=()

run_gate() {
  local name="$1"
  local command="$2"
  local log_slug="$3"
  local log_file="tmp/stage5a-evidence-logs/${log_slug}.log"

  echo "==> ${name}"
  set +e
  bash -lc "$command" >"$log_file" 2>&1
  local status=$?
  set -e

  LOG_FILES+=("$log_file")
  if [[ $status -eq 0 ]]; then
    echo "PASS ${name}"
    RESULT_ROWS+=("| ${name} | PASS | \`${command//|/\\|}\` | \`${log_file}\` |")
  else
    echo "FAIL ${name}; see ${log_file}" >&2
    RESULT_ROWS+=("| ${name} | FAIL | \`${command//|/\\|}\` | \`${log_file}\` |")
    write_report "$started_at" "failed"
    exit $status
  fi
}

write_report() {
  local report_started_at="$1"
  local status="$2"
  local finished_at
  finished_at="$(date -u +%Y-%m-%dT%H:%M:%SZ)"

  {
    echo "# Stage 5A Release Evidence"
    echo
    echo "Generated: ${finished_at}"
    echo "Started: ${report_started_at}"
    echo "Status: ${status}"
    echo
    echo "## Repository"
    echo
    echo "- Branch: \`${branch}\`"
    echo "- Commit: \`${commit}\`"
    echo "- Full commit: \`${full_commit}\`"
    echo
    echo "## Gate Results"
    echo
    echo "| Gate | Result | Command | Log |"
    echo "|---|---|---|---|"
    printf '%s\n' "${RESULT_ROWS[@]}"
    echo
    echo "## Contract Coverage"
    echo
    echo "- Sync bridge reducer / FFI contract: \`scripts/smoke-stage5a-sync-bridge.sh\`"
    echo "- Bundled FFI artifact architecture / symbols / version: \`scripts/check-ffi-artifacts.sh\`"
    echo "- SAGE policy bundle / invocation / report / governance runtime outcomes: \`scripts/check-sage-client-runtime-contract.sh\`"
    echo "- HTTP governance error envelope: \`scripts/check-governance-client-error-contract.sh\`"
    echo "- Default local release path: \`scripts/release-gate-local.sh\`"
    if [[ "$RUN_SDK_TESTS" == "1" ]]; then
      echo "- SDK targeted contract tests: \`cargo test -p agentos-client-bridge -p agentos-ffi --locked\`"
    else
      echo "- SDK targeted contract tests: skipped by default; rerun with \`RUN_SDK_TESTS=1\`."
    fi
    echo
    echo "## Optional Live Gates"
    echo
    echo "Run these with a local stack when validating environment-backed behavior:"
    echo
    echo '```bash'
    echo 'RUN_MIGRATION_GATE=1 RUN_LIVE_SMOKES=1 RUN_LOCAL_HTTP_SEMANTIC=1 ./scripts/release-gate-local.sh'
    echo '```'
  } >"$OUTPUT_PATH"
}

run_gate "Go tests" "go test ./..." "go-test-all"
run_gate "FFI artifact contract" "./scripts/check-ffi-artifacts.sh" "ffi-artifacts"
run_gate "Stage 5A sync bridge contract" "./scripts/smoke-stage5a-sync-bridge.sh" "sync-bridge"
run_gate "SAGE client runtime contract" "./scripts/check-sage-client-runtime-contract.sh" "sage-client-runtime"
run_gate "Governance client error contract" "./scripts/check-governance-client-error-contract.sh" "governance-client-error"
run_gate "Local release gate" "./scripts/release-gate-local.sh" "release-gate-local"

if [[ "$RUN_SDK_TESTS" == "1" ]]; then
  run_gate "SDK targeted FFI/client bridge tests" "cd '$SDK_ROOT' && cargo test -p agentos-client-bridge -p agentos-ffi --locked" "sdk-targeted-tests"
fi

write_report "$started_at" "passed"

echo
printf 'Stage 5A release evidence written to %s\n' "$OUTPUT_PATH"
