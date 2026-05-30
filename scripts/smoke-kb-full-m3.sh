#!/usr/bin/env bash
set -euo pipefail

BASE_URL="${BASE_URL:-http://localhost:8080}"
RUN_ID="${RUN_ID:-$(date +%s)}"
PUBLISHER_DEVICE_ID="${PUBLISHER_DEVICE_ID:-kb-m3-publisher-${RUN_ID}}"
CONSUMER_DEVICE_ID="${CONSUMER_DEVICE_ID:-kb-m3-consumer-${RUN_ID}}"
ENTRY_ID="notes/kb-full-m3-${RUN_ID}"

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

make_token() {
  local device_id="$1"
  local identity key pubkey
  identity="$(new_identity "$device_id")"
  key="${identity%%|*}"
  pubkey="${identity##*|}"
  verify_device "$device_id" "$key" "$pubkey"
}

echo "==> Health check ${BASE_URL}/health"
api GET /health '' '' 200 | jq '{status, checks, identity_verifier}'

echo "==> Verify publisher and consumer identities"
PUBLISHER_TOKEN="$(make_token "$PUBLISHER_DEVICE_ID")"
CONSUMER_TOKEN="$(make_token "$CONSUMER_DEVICE_ID")"

echo "==> Publisher creates knowledge entry, collection, pricing, and snapshot"
api POST /api/v1/knowledge/entries "$PUBLISHER_TOKEN" "$(jq -cn --arg entry_id "$ENTRY_ID" '{entry_id:$entry_id,title:"Full M3 Searchable Strategy",content_markdown:"# Full M3 Searchable Strategy\n\nRevenue and marketplace metering content.",summary:"full M3 searchable strategy summary",tags:["kb","m3","strategy"],metadata:{smoke:true},client_event_id:"kb-full-m3-create-1"}')" 201 >/dev/null
collection_response="$(api POST /api/v1/kb/collections "$PUBLISHER_TOKEN" "$(jq -cn --arg name "Full M3 KB ${RUN_ID}" '{name:$name,description:"full M3 marketplace smoke"}')" 201)"
collection_id="$(echo "$collection_response" | jq -r '.data.id')"
api PUT "/api/v1/kb/collections/${collection_id}/pricing" "$PUBLISHER_TOKEN" '{"is_free":false,"pricing_model":"monthly","monthly_price":9900,"platform_min_price":100,"platform_max_price":20000}' 200 >/dev/null
snapshot_response="$(api POST "/api/v1/kb/collections/${collection_id}/snapshots" "$PUBLISHER_TOKEN" "$(jq -cn --arg entry_id "$ENTRY_ID" '{entry_ids:[$entry_id]}')" 201)"
snapshot_id="$(echo "$snapshot_response" | jq -r '.data.snapshot.id')"
entry_record_id="$(echo "$snapshot_response" | jq -r '.data.entries[0].id')"

echo "==> Public marketplace search and lexical search"
market_response="$(api GET "/api/v1/kb/public/collections?q=Full%20M3&limit=10" '' '' 200)"
if [[ "$(echo "$market_response" | jq -r --arg id "$collection_id" '.data.items[] | select(.collection_id==$id) | .monthly_price' | head -1)" != "9900" ]]; then
  echo "marketplace search did not return priced collection" >&2
  echo "$market_response" | jq >&2
  exit 1
fi
search_response="$(api GET "/api/v1/kb/public/search?q=Searchable&mode=lexical&collection_id=${collection_id}" '' '' 200)"
if [[ "$(echo "$search_response" | jq -r '.data.items[0].entry_id')" != "$ENTRY_ID" ]]; then
  echo "lexical search did not return expected entry" >&2
  echo "$search_response" | jq >&2
  exit 1
fi
api GET "/api/v1/kb/public/search?q=Searchable&mode=semantic" '' '' 501 >/dev/null

echo "==> Consumer installs and accesses manifest/content/fulltext"
api POST "/api/v1/kb/collections/${collection_id}/install" "$CONSUMER_TOKEN" '{"track_mode":"latest"}' 201 >/dev/null
api POST "/api/v1/kb/collections/${collection_id}/snapshots/${snapshot_id}/manifest-download-url" "$CONSUMER_TOKEN" '' 200 >/dev/null
api POST "/api/v1/kb/collections/${collection_id}/snapshots/${snapshot_id}/entries/${entry_record_id}/content-download-url" "$CONSUMER_TOKEN" '' 200 >/dev/null
fulltext_response="$(api POST "/api/v1/kb/collections/${collection_id}/snapshots/${snapshot_id}/entries/${entry_record_id}/fulltext" "$CONSUMER_TOKEN" '' 200)"
if [[ "$(echo "$fulltext_response" | jq -r '.data.usage.operation_type')" != "entry.fulltext.fetch" ]]; then
  echo "fulltext usage missing" >&2
  echo "$fulltext_response" | jq >&2
  exit 1
fi

echo "==> Owner stats and billing APIs"
stats_response="$(api GET "/api/v1/kb/collections/${collection_id}/stats" "$PUBLISHER_TOKEN" '' 200)"
if [[ "$(echo "$stats_response" | jq -r '.data.active_install_count')" != "1" ]]; then
  echo "stats active install count mismatch" >&2
  echo "$stats_response" | jq >&2
  exit 1
fi
if [[ "$(echo "$stats_response" | jq -r '.data.manifest_download_count')" -lt "1" || "$(echo "$stats_response" | jq -r '.data.content_download_count')" -lt "1" ]]; then
  echo "stats usage counts mismatch" >&2
  echo "$stats_response" | jq >&2
  exit 1
fi
api GET /api/v1/billing/account "$CONSUMER_TOKEN" '' 200 >/dev/null
api GET /api/v1/billing/transactions "$CONSUMER_TOKEN" '' 200 >/dev/null
api GET "/api/v1/kb/collections/${collection_id}/earnings" "$PUBLISHER_TOKEN" '' 200 >/dev/null

echo
echo "KB full M3 smoke passed."
echo "  Collection ID: ${collection_id}"
echo "  Snapshot ID:   ${snapshot_id}"
echo "  Entry Record:  ${entry_record_id}"
