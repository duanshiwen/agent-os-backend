#!/usr/bin/env bash
set -euo pipefail

BASE_URL="${BASE_URL:-http://localhost:8080}"
RUN_ID="${RUN_ID:-$(date +%s)}"
PUBLISHER_DEVICE_ID="${PUBLISHER_DEVICE_ID:-kb-publisher-${RUN_ID}}"
CONSUMER_DEVICE_ID="${CONSUMER_DEVICE_ID:-kb-consumer-${RUN_ID}}"
ENTRY_ID="notes/kb-install-${RUN_ID}"

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
  verify_body="$(jq -cn \
    --arg device_id "$device_id" \
    --arg user_pubkey "$pubkey" \
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

publisher_identity="$(new_identity "$PUBLISHER_DEVICE_ID")"
publisher_key="${publisher_identity%%|*}"
publisher_pubkey="${publisher_identity##*|}"
consumer_identity="$(new_identity "$CONSUMER_DEVICE_ID")"
consumer_key="${consumer_identity%%|*}"
consumer_pubkey="${consumer_identity##*|}"

echo "==> Verify publisher and consumer identities"
PUBLISHER_TOKEN="$(verify_device "$PUBLISHER_DEVICE_ID" "$publisher_key" "$publisher_pubkey")"
CONSUMER_TOKEN="$(verify_device "$CONSUMER_DEVICE_ID" "$consumer_key" "$consumer_pubkey")"

echo "==> Publisher creates source knowledge entry"
create_body="$(jq -cn --arg entry_id "$ENTRY_ID" '{entry_id:$entry_id,title:"KB Install Smoke",content_markdown:"# KB Install Smoke\n\nConsumer install content.",summary:"install smoke",tags:["kb","install"],metadata:{smoke:true},client_event_id:"kb-install-create-1"}')"
api POST /api/v1/knowledge/entries "$PUBLISHER_TOKEN" "$create_body" 201 >/dev/null

echo "==> Publisher creates collection and snapshot"
collection_response="$(api POST /api/v1/kb/collections "$PUBLISHER_TOKEN" "$(jq -cn --arg name "Install Smoke KB ${RUN_ID}" '{name:$name,description:"consumer install smoke"}')" 201)"
collection_id="$(echo "$collection_response" | jq -r '.data.id')"
snapshot_response="$(api POST "/api/v1/kb/collections/${collection_id}/snapshots" "$PUBLISHER_TOKEN" "$(jq -cn --arg entry_id "$ENTRY_ID" '{entry_ids:[$entry_id]}')" 201)"
snapshot_id="$(echo "$snapshot_response" | jq -r '.data.snapshot.id')"
version="$(echo "$snapshot_response" | jq -r '.data.snapshot.version')"
if [[ "$version" != "1" ]]; then
  echo "expected snapshot v1, got $version" >&2
  echo "$snapshot_response" | jq >&2
  exit 1
fi

echo "==> Public discovery and snapshot detail"
public_list="$(api GET /api/v1/kb/public/collections '' '' 200)"
if ! echo "$public_list" | jq -e --arg id "$collection_id" '.data[] | select(.id == $id)' >/dev/null; then
  echo "published collection not found in public list" >&2
  echo "$public_list" | jq >&2
  exit 1
fi
public_detail="$(api GET "/api/v1/kb/public/collections/${collection_id}" '' '' 200)"
latest_snapshot="$(echo "$public_detail" | jq -r '.data.latest_snapshot.id')"
if [[ "$latest_snapshot" != "$snapshot_id" ]]; then
  echo "public latest snapshot mismatch" >&2
  echo "$public_detail" | jq >&2
  exit 1
fi
api GET "/api/v1/kb/public/collections/${collection_id}/snapshots/${snapshot_id}" '' '' 200 >/dev/null

echo "==> Public manifest download URL"
manifest_response="$(api POST "/api/v1/kb/public/collections/${collection_id}/snapshots/${snapshot_id}/manifest-download-url" '' '' 200)"
manifest_url="$(echo "$manifest_response" | jq -r '.data.download_url')"
if [[ -z "$manifest_url" || "$manifest_url" == "null" ]]; then
  echo "manifest download url missing" >&2
  echo "$manifest_response" | jq >&2
  exit 1
fi
manifest_file="$KEY_DIR/manifest.json"
curl -sS "$manifest_url" -o "$manifest_file"
if [[ "$(jq -r '.snapshot_id' "$manifest_file")" != "$snapshot_id" ]]; then
  echo "downloaded manifest snapshot id mismatch" >&2
  cat "$manifest_file" >&2
  exit 1
fi

echo "==> Consumer installs latest and pinned"
install_latest="$(api POST "/api/v1/kb/collections/${collection_id}/install" "$CONSUMER_TOKEN" '{"track_mode":"latest"}' 201)"
if [[ "$(echo "$install_latest" | jq -r '.data.track_mode')" != "latest" ]]; then
  echo "latest install failed" >&2
  echo "$install_latest" | jq >&2
  exit 1
fi
install_pinned="$(api POST "/api/v1/kb/collections/${collection_id}/install" "$CONSUMER_TOKEN" '{"track_mode":"pinned","pinned_version":1}' 201)"
if [[ "$(echo "$install_pinned" | jq -r '.data.track_mode')" != "pinned" ]]; then
  echo "pinned install failed" >&2
  echo "$install_pinned" | jq >&2
  exit 1
fi
subscriptions="$(api GET /api/v1/kb/subscriptions "$CONSUMER_TOKEN" '' 200)"
if [[ "$(echo "$subscriptions" | jq '[.data[] | select(.collection_id == "'"$collection_id"'")] | length')" != "1" ]]; then
  echo "expected one consumer subscription" >&2
  echo "$subscriptions" | jq >&2
  exit 1
fi

echo
echo "KB install smoke passed."
echo "  Collection ID: ${collection_id}"
echo "  Snapshot ID:   ${snapshot_id}"
echo "  Consumer:      ${CONSUMER_DEVICE_ID}"
