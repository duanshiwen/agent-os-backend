#!/usr/bin/env bash
set -euo pipefail

BASE_URL="${BASE_URL:-http://localhost:8080}"
TOKEN="${TOKEN:-}"
AFTER_SEQUENCE="${AFTER_SEQUENCE:-0}"
LIMIT="${LIMIT:-100}"
ACK_SEQUENCE="${ACK_SEQUENCE:-}"

if [[ -z "$TOKEN" ]]; then
  cat >&2 <<'USAGE'
TOKEN is required.

Example:
  TOKEN="<jwt>" ./scripts/smoke-sync.sh

Optional environment variables:
  BASE_URL=http://localhost:8080
  AFTER_SEQUENCE=0
  LIMIT=100
  ACK_SEQUENCE=123

If ACK_SEQUENCE is omitted, the script only pulls events and validates that the
sync endpoint returns a response. If jq is installed and the pull response has
next_after_sequence, that value is printed as a suggested ACK_SEQUENCE.
USAGE
  exit 2
fi

pull_url="${BASE_URL}/api/v1/sync/events?after_sequence=${AFTER_SEQUENCE}&limit=${LIMIT}"

echo "==> Pull sync events"
echo "GET ${pull_url}"
pull_response="$({
  curl -fsS \
    -H "Authorization: Bearer ${TOKEN}" \
    "$pull_url"
} 2>&1)" || {
  status=$?
  echo "Sync pull failed:" >&2
  echo "$pull_response" >&2
  exit "$status"
}

echo "$pull_response"

if command -v jq >/dev/null 2>&1; then
  echo
  echo "==> Pull summary"
  echo "$pull_response" | jq '{code, message, events: (.data.events | length), next_after_sequence: .data.next_after_sequence, has_more: .data.has_more, schema_version: .data.schema_version}'

  suggested_ack="$(echo "$pull_response" | jq -r '.data.next_after_sequence // empty')"
  if [[ -z "$ACK_SEQUENCE" && -n "$suggested_ack" && "$suggested_ack" != "0" ]]; then
    echo
    echo "Suggested ack: ACK_SEQUENCE=${suggested_ack} ./scripts/smoke-sync.sh"
  fi
fi

if [[ -n "$ACK_SEQUENCE" ]]; then
  echo
  echo "==> Ack sync events"
  echo "POST ${BASE_URL}/api/v1/sync/ack last_sequence=${ACK_SEQUENCE}"
  curl -fsS \
    -X POST \
    -H "Authorization: Bearer ${TOKEN}" \
    -H "Content-Type: application/json" \
    -d "{\"last_sequence\":${ACK_SEQUENCE}}" \
    "${BASE_URL}/api/v1/sync/ack"
  echo
fi
