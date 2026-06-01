#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT_DIR"

require_tool() {
  if ! command -v "$1" >/dev/null 2>&1; then
    echo "missing required tool: $1" >&2
    exit 1
  fi
}

require_tool file
require_tool nm

required_symbols=(
  agentos_ffi_version
  agentos_ffi_free_string
  agentos_identity_verify_ed25519_challenge
  agentos_apply_knowledge_sync_events_json
  agentos_apply_knowledge_sync_pull_response_json
  agentos_apply_sync_pull_response_json
)

artifacts=(
  "darwin-arm64:internal/runtime/darwin-arm64/libagentos_ffi.dylib:Mach-O 64-bit dynamically linked shared library arm64"
  "linux-arm64:internal/runtime/linux-arm64/libagentos_ffi.so:ELF 64-bit LSB shared object, ARM aarch64"
)

extract_symbols() {
  local artifact="$1"
  nm -gU "$artifact" 2>/dev/null \
    || nm -D "$artifact" 2>/dev/null \
    || nm -g "$artifact" 2>/dev/null
}

normalize_symbols() {
  awk '{print $NF}' | sed 's/^_//' | grep '^agentos_' | sort -u
}

echo "==> FFI bundled artifact checks"
for spec in "${artifacts[@]}"; do
  IFS=: read -r platform artifact expected_file_fragment <<<"$spec"
  echo "--> $platform $artifact"

  if [[ ! -f "$artifact" ]]; then
    echo "missing bundled FFI artifact: $artifact" >&2
    exit 1
  fi

  file_output="$(file "$artifact")"
  echo "$file_output"
  if [[ "$file_output" != *"$expected_file_fragment"* ]]; then
    echo "unexpected artifact type for $artifact; expected to contain: $expected_file_fragment" >&2
    exit 1
  fi

  symbols="$(extract_symbols "$artifact" | normalize_symbols)"
  for symbol in "${required_symbols[@]}"; do
    if ! grep -qx "$symbol" <<<"$symbols"; then
      echo "missing required FFI export $symbol in $artifact" >&2
      echo "exported agentos symbols:" >&2
      echo "$symbols" >&2
      exit 1
    fi
  done
done

# This loads only the current-platform artifact and verifies runtime ABI/version behavior.
echo "==> FFI current-platform artifact contract"
go test ./internal/service -run 'TestFFIArtifactContractIntegration' -count=1 -v

printf '\nFFI bundled artifact checks passed.\n'
