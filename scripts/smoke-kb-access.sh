#!/usr/bin/env bash
set -euo pipefail

BASE_URL="${BASE_URL:-http://localhost:8080}"
RUN_ID="${RUN_ID:-$(date +%s)}"
PUBLISHER_DEVICE_ID="${PUBLISHER_DEVICE_ID:-kb-access-publisher-${RUN_ID}}"
CONSUMER_DEVICE_ID="${CONSUMER_DEVICE_ID:-kb-access-consumer-${RUN_ID}}"
STRANGER_DEVICE_ID="${STRANGER_DEVICE_ID:-kb-access-stranger-${RUN_ID}}"
ENTRY_ID="notes/kb-access-${RUN_ID}"

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

new_identity() {
  local device_id="$1"
  local key_file="$KEY_DIR/${device_id}.pem"
  openssl genpkey -algorithm Ed25519 -out "$key_file" >/dev/null 2>&1
  local pubkey
  pubkey="$(openssl pkey -in "$key_file" -pubout -outform DER 2>/dev/null | xxd -p -c 256 | sed 's/^.*032100//')"
  echo "$key_file|$pubkey"
}

sign_hex() {
  local key_file="$1"
  local message="$2"
  local message_file signature_file
  message_file="$(mktemp "$KEY_DIR/message.XXXXXX")"
  signature_file="$(mktemp "$KEY_DIR/signature.XXXXXX")"
  printf '%s' "$message" > "$message_file"
  openssl pkeyutl -sign -rawin -inkey "$key_file" -in "$message_file" -out "$signature_file"
  xxd -p -c 256 "$signature_file"
}

verify_device() {
  local device_id="$1"
  local key_file="$2"
  local pubkey="$3"
  local challenge nonce signature verify_body verify_response token
  challenge="$(api POST /api/v1/auth/challenge '' "$(jq -cn --arg device_id "$device_id" --arg user_pubkey "$pubkey" '{device_id:$device_id,user_pubkey:$user_pubkey}')" 200)"
  nonce="$(echo "$challenge" | jq -r '.data.nonce')"
  signature="$(sign_hex "$key_file" "$(echo "$challenge" | jq -r '.data.challenge')")"
  verify_body="$(jq -cn --arg device_id "$device_id" --arg user_pubkey "$pubkey" --arg nonce "$nonce" --arg signature "$signature" '{device_id:$device_id,user_pubkey:$user_pubkey,nonce:$nonce,signature:$signature}')"
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

make_token() {
  local device_id="$1"
  local identity key pubkey
  identity="$(new_identity "$device_id")"
  key="${identity%%|*}"
  pubkey="${identity##*|}"
  verify_device "$device_id" "$key" "$pubkey"
}

echo "==> Verify publisher, consumer, and stranger identities"
PUBLISHER_TOKEN="$(make_token "$PUBLISHER_DEVICE_ID")"
CONSUMER_TOKEN="$(make_token "$CONSUMER_DEVICE_ID")"
STRANGER_TOKEN="$(make_token "$STRANGER_DEVICE_ID")"

echo "==> Publisher creates collection snapshot"
api POST /api/v1/knowledge/entries "$PUBLISHER_TOKEN" "$(jq -cn --arg entry_id "$ENTRY_ID" '{entry_id:$entry_id,title:"KB Access Smoke",content_markdown:"# KB Access Smoke\n\nInstalled content access.",summary:"access smoke",tags:["kb","access"],metadata:{smoke:true},client_event_id:"kb-access-create-1"}')" 201 >/dev/null
collection_response="$(api POST /api/v1/kb/collections "$PUBLISHER_TOKEN" "$(jq -cn --arg name "Access Smoke KB ${RUN_ID}" '{name:$name,description:"consumer access smoke"}')" 201)"
collection_id="$(echo "$collection_response" | jq -r '.data.id')"
snapshot_response="$(api POST "/api/v1/kb/collections/${collection_id}/snapshots" "$PUBLISHER_TOKEN" "$(jq -cn --arg entry_id "$ENTRY_ID" '{entry_ids:[$entry_id]}')" 201)"
snapshot_id="$(echo "$snapshot_response" | jq -r '.data.snapshot.id')"
entry_record_id="$(echo "$snapshot_response" | jq -r '.data.entries[0].id')"

echo "==> Public metadata remains readable"
api GET "/api/v1/kb/public/collections/${collection_id}/snapshots/${snapshot_id}" '' '' 200 >/dev/null

echo "==> Stranger cannot access installed content endpoints"
api POST "/api/v1/kb/collections/${collection_id}/snapshots/${snapshot_id}/manifest-download-url" "$STRANGER_TOKEN" '' 403 >/dev/null
api POST "/api/v1/kb/collections/${collection_id}/snapshots/${snapshot_id}/entries/${entry_record_id}/content-download-url" "$STRANGER_TOKEN" '' 403 >/dev/null

echo "==> Consumer installs and accesses manifest/content"
api POST "/api/v1/kb/collections/${collection_id}/install" "$CONSUMER_TOKEN" '{"track_mode":"latest"}' 201 >/dev/null
manifest_response="$(api POST "/api/v1/kb/collections/${collection_id}/snapshots/${snapshot_id}/manifest-download-url" "$CONSUMER_TOKEN" '' 200)"
content_response="$(api POST "/api/v1/kb/collections/${collection_id}/snapshots/${snapshot_id}/entries/${entry_record_id}/content-download-url" "$CONSUMER_TOKEN" '' 200)"
manifest_url="$(echo "$manifest_response" | jq -r '.data.download_url')"
content_url="$(echo "$content_response" | jq -r '.data.download_url')"
if [[ -z "$manifest_url" || "$manifest_url" == "null" || -z "$content_url" || "$content_url" == "null" ]]; then
  echo "download URL missing" >&2
  echo "$manifest_response" | jq >&2
  echo "$content_response" | jq >&2
  exit 1
fi
manifest_file="$KEY_DIR/manifest.json"
content_file="$KEY_DIR/content.md"
curl -sS "$manifest_url" -o "$manifest_file"
curl -sS "$content_url" -o "$content_file"
if [[ "$(jq -r '.snapshot_id' "$manifest_file")" != "$snapshot_id" ]]; then
  echo "manifest snapshot mismatch" >&2
  cat "$manifest_file" >&2
  exit 1
fi
if ! grep -q "Installed content access" "$content_file"; then
  echo "downloaded content mismatch" >&2
  cat "$content_file" >&2
  exit 1
fi

echo "==> Cancel subscription and verify access denied"
api DELETE "/api/v1/kb/collections/${collection_id}/install" "$CONSUMER_TOKEN" '' 200 >/dev/null
api POST "/api/v1/kb/collections/${collection_id}/snapshots/${snapshot_id}/entries/${entry_record_id}/content-download-url" "$CONSUMER_TOKEN" '' 403 >/dev/null

echo
echo "KB access smoke passed."
echo "  Collection ID: ${collection_id}"
echo "  Snapshot ID:   ${snapshot_id}"
echo "  Entry Record:  ${entry_record_id}"
