#!/usr/bin/env bash
set -euo pipefail

BASE_URL="${BASE_URL:-http://localhost:8080}"
RUN_ID="${RUN_ID:-$(date +%s)}"
DEVICE_ID="${DEVICE_ID:-kb-snapshot-smoke-device-${RUN_ID}}"
ENTRY_ID="notes/kb-snapshot-${RUN_ID}"

require_tool() {
  if ! command -v "$1" >/dev/null 2>&1; then
    echo "missing required tool: $1" >&2
    exit 1
  fi
}

require_tool curl
require_tool jq
require_tool openssl
require_tool xxd

api() {
  local method="$1"
  local path="$2"
  local token="${3:-}"
  local body="${4:-}"
  local expected_status="${5:-}"
  local response_file status
  response_file="$(mktemp)"
  if [[ -n "$body" ]]; then
    status="$(curl -sS -o "$response_file" -w '%{http_code}' \
      -X "$method" \
      -H 'Content-Type: application/json' \
      ${token:+-H "Authorization: Bearer ${token}"} \
      -d "$body" \
      "${BASE_URL}${path}")"
  else
    status="$(curl -sS -o "$response_file" -w '%{http_code}' \
      -X "$method" \
      ${token:+-H "Authorization: Bearer ${token}"} \
      "${BASE_URL}${path}")"
  fi

  if [[ -n "$expected_status" && "$status" != "$expected_status" ]]; then
    echo "unexpected HTTP status for ${method} ${path}: got ${status}, expected ${expected_status}" >&2
    cat "$response_file" >&2
    rm -f "$response_file"
    exit 1
  fi
  cat "$response_file"
  rm -f "$response_file"
}

KEY_DIR="$(mktemp -d)"
trap 'rm -rf "$KEY_DIR"' EXIT
KEY_FILE="$KEY_DIR/device.pem"
openssl genpkey -algorithm Ed25519 -out "$KEY_FILE" >/dev/null 2>&1
PUBKEY="$(openssl pkey -in "$KEY_FILE" -pubout -outform DER 2>/dev/null | xxd -p -c 256 | sed 's/^.*032100//')"

sign_hex() {
  local message="$1"
  local message_file signature_file
  message_file="$(mktemp "$KEY_DIR/message.XXXXXX")"
  signature_file="$(mktemp "$KEY_DIR/signature.XXXXXX")"
  printf '%s' "$message" > "$message_file"
  openssl pkeyutl -sign -rawin -inkey "$KEY_FILE" -in "$message_file" -out "$signature_file"
  xxd -p -c 256 "$signature_file"
}

verify_device() {
  local challenge nonce signature verify_body verify_response token
  challenge="$(api POST /api/v1/auth/challenge '' "$(jq -cn --arg device_id "$DEVICE_ID" --arg user_pubkey "$PUBKEY" '{device_id:$device_id,user_pubkey:$user_pubkey}')" 200)"
  nonce="$(echo "$challenge" | jq -r '.data.nonce')"
  signature="$(sign_hex "$(echo "$challenge" | jq -r '.data.challenge')")"
  verify_body="$(jq -cn \
    --arg device_id "$DEVICE_ID" \
    --arg user_pubkey "$PUBKEY" \
    --arg nonce "$nonce" \
    --arg signature "$signature" \
    '{device_id:$device_id,user_pubkey:$user_pubkey,nonce:$nonce,signature:$signature}')"
  verify_response="$(api POST /api/v1/auth/verify '' "$verify_body" 200)"
  token="$(echo "$verify_response" | jq -r '.data.access_token')"
  if [[ -z "$token" || "$token" == "null" ]]; then
    echo "auth verify did not return access token" >&2
    echo "$verify_response" >&2
    exit 1
  fi
  echo "$token"
}

echo "==> Health check ${BASE_URL}/health"
api GET /health '' '' 200 | jq '{status, checks, identity_verifier}'

echo "==> Verify smoke device ${DEVICE_ID}"
TOKEN="$(verify_device)"

echo "==> Create source knowledge entry ${ENTRY_ID}"
create_body="$(jq -cn --arg entry_id "$ENTRY_ID" '{entry_id:$entry_id,title:"KB Snapshot Smoke v1",content_markdown:"# KB Snapshot Smoke\n\nVersion one content.",summary:"snapshot smoke v1",tags:["kb","snapshot"],metadata:{smoke:true},client_event_id:"kb-snapshot-create-1"}')"
api POST /api/v1/knowledge/entries "$TOKEN" "$create_body" 201 >/dev/null

echo "==> Create KB collection"
collection_response="$(api POST /api/v1/kb/collections "$TOKEN" "$(jq -cn --arg name "Smoke KB ${RUN_ID}" '{name:$name,description:"KB snapshot smoke collection"}')" 201)"
collection_id="$(echo "$collection_response" | jq -r '.data.id')"
if [[ -z "$collection_id" || "$collection_id" == "null" ]]; then
  echo "collection id missing" >&2
  echo "$collection_response" | jq >&2
  exit 1
fi

echo "==> Publish snapshot v1"
snapshot_v1="$(api POST "/api/v1/kb/collections/${collection_id}/snapshots" "$TOKEN" "$(jq -cn --arg entry_id "$ENTRY_ID" '{entry_ids:[$entry_id]}')" 201)"
snapshot_id="$(echo "$snapshot_v1" | jq -r '.data.snapshot.id')"
version="$(echo "$snapshot_v1" | jq -r '.data.snapshot.version')"
manifest_uri="$(echo "$snapshot_v1" | jq -r '.data.snapshot.manifest_object_uri')"
entry_title="$(echo "$snapshot_v1" | jq -r '.data.entries[0].title')"
content_uri="$(echo "$snapshot_v1" | jq -r '.data.entries[0].content_object_uri')"
if [[ "$version" != "1" || "$entry_title" != "KB Snapshot Smoke v1" || -z "$manifest_uri" || "$manifest_uri" == "null" || -z "$content_uri" || "$content_uri" == "null" ]]; then
  echo "unexpected snapshot v1" >&2
  echo "$snapshot_v1" | jq >&2
  exit 1
fi

echo "==> Mutate source knowledge entry after snapshot"
update_body="$(jq -cn '{title:"KB Snapshot Smoke v2",content_markdown:"# KB Snapshot Smoke\n\nVersion two content.",summary:"snapshot smoke v2",tags:["kb","snapshot"],metadata:{smoke:true,version:2},client_event_id:"kb-snapshot-update-1",base_version:1}')"
api PUT "/api/v1/knowledge/entries/${ENTRY_ID}" "$TOKEN" "$update_body" 200 >/dev/null

echo "==> Verify snapshot v1 remains immutable"
loaded_v1="$(api GET "/api/v1/kb/collections/${collection_id}/snapshots/${snapshot_id}" "$TOKEN" '' 200)"
loaded_title="$(echo "$loaded_v1" | jq -r '.data.entries[0].title')"
if [[ "$loaded_title" != "KB Snapshot Smoke v1" ]]; then
  echo "snapshot v1 mutated unexpectedly; got title ${loaded_title}" >&2
  echo "$loaded_v1" | jq >&2
  exit 1
fi

echo "==> Publish snapshot v2"
snapshot_v2="$(api POST "/api/v1/kb/collections/${collection_id}/snapshots" "$TOKEN" "$(jq -cn --arg entry_id "$ENTRY_ID" '{entry_ids:[$entry_id]}')" 201)"
version2="$(echo "$snapshot_v2" | jq -r '.data.snapshot.version')"
title2="$(echo "$snapshot_v2" | jq -r '.data.entries[0].title')"
if [[ "$version2" != "2" || "$title2" != "KB Snapshot Smoke v2" ]]; then
  echo "unexpected snapshot v2" >&2
  echo "$snapshot_v2" | jq >&2
  exit 1
fi

echo
echo "KB snapshot smoke passed."
echo "  Collection ID: ${collection_id}"
echo "  Snapshot v1:   ${snapshot_id}"
echo "  Manifest URI:  ${manifest_uri}"
echo "  Content URI:   ${content_uri}"
