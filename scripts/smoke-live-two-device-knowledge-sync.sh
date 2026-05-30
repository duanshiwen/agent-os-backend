#!/usr/bin/env bash
set -euo pipefail

BASE_URL="${BASE_URL:-http://localhost:8080}"
RUN_ID="${RUN_ID:-$(date +%s)}"
DEVICE_A="${DEVICE_A:-live-knowledge-a-${RUN_ID}}"
DEVICE_B="${DEVICE_B:-live-knowledge-b-${RUN_ID}}"
ENTRY_ID="${ENTRY_ID:-notes/live-two-device-${RUN_ID}}"

require_tool() {
  if ! command -v "$1" >/dev/null 2>&1; then
    echo "missing required tool: $1" >&2
    exit 1
  fi
}

require_tool curl
require_tool jq
require_tool openssl
require_tool python3
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
KEY_A_FILE="$KEY_DIR/device-a.pem"
KEY_B_FILE="$KEY_DIR/device-b.pem"

make_key() {
  local key_file="$1"
  openssl genpkey -algorithm Ed25519 -out "$key_file" >/dev/null 2>&1
}

pubkey_hex() {
  local key_file="$1"
  openssl pkey -in "$key_file" -pubout -outform DER 2>/dev/null | xxd -p -c 256 | sed 's/^.*032100//'
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

canonical_pairing_claim() {
  local qr_payload="$1"
  local new_device_id="$2"
  local new_device_pubkey="$3"
  QR_PAYLOAD="$qr_payload" NEW_DEVICE_ID="$new_device_id" NEW_DEVICE_PUBKEY="$new_device_pubkey" python3 - <<'PY'
import base64, json, os
payload = os.environ["QR_PAYLOAD"]
padding = "=" * ((4 - len(payload) % 4) % 4)
qr = json.loads(base64.urlsafe_b64decode((payload + padding).encode()))
claim = {
  "expires_at": qr["expires_at"],
  "new_device_id": os.environ["NEW_DEVICE_ID"],
  "new_device_pubkey": os.environ["NEW_DEVICE_PUBKEY"],
  "pairing_session_id": qr["pairing_session_id"],
  "purpose": "agentos.device_pairing.claim",
  "server_id": qr["server_id"],
  "version": qr["version"],
}
print(json.dumps(claim, separators=(",", ":"), sort_keys=True))
PY
}

verify_device() {
  local device_id="$1"
  local pubkey="$2"
  local key_file="$3"

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

assert_event() {
  local response="$1"
  local expected_type="$2"
  local expected_entry="$3"
  local expected_count="${4:-1}"
  local count event_type entry_id
  count="$(echo "$response" | jq '.data.events | length')"
  if [[ "$count" != "$expected_count" ]]; then
    echo "expected ${expected_count} event(s), got ${count}" >&2
    echo "$response" | jq >&2
    exit 1
  fi
  if [[ "$expected_count" == "0" ]]; then
    return
  fi
  event_type="$(echo "$response" | jq -r '.data.events[0].event_type')"
  entry_id="$(echo "$response" | jq -r '.data.events[0].payload.entry_id')"
  if [[ "$event_type" != "$expected_type" || "$entry_id" != "$expected_entry" ]]; then
    echo "unexpected event: type=${event_type} entry=${entry_id}" >&2
    echo "$response" | jq >&2
    exit 1
  fi
}

ack_pull() {
  local token="$1"
  local pull_response="$2"
  local seq
  seq="$(echo "$pull_response" | jq -r '.data.next_after_sequence')"
  if [[ -z "$seq" || "$seq" == "null" || "$seq" == "0" ]]; then
    echo "cannot ack empty sequence from pull response" >&2
    echo "$pull_response" | jq >&2
    exit 1
  fi
  api POST /api/v1/sync/ack "$token" "$(jq -cn --argjson last_sequence "$seq" '{last_sequence:$last_sequence}')" 200 >/dev/null
}

echo "==> Health check ${BASE_URL}/health"
api GET /health '' '' 200 | jq '{status, checks, identity_verifier}'

make_key "$KEY_A_FILE"
make_key "$KEY_B_FILE"
PUB_A="$(pubkey_hex "$KEY_A_FILE")"
PUB_B="$(pubkey_hex "$KEY_B_FILE")"

echo "==> Verify Device A (${DEVICE_A})"
TOKEN_A="$(verify_device "$DEVICE_A" "$PUB_A" "$KEY_A_FILE")"

echo "==> QR-only pair Device B (${DEVICE_B})"
start_response="$(api POST /api/v1/devices/pairing/start "$TOKEN_A" '{}' 201)"
qr_payload="$(echo "$start_response" | jq -r '.data.qr_payload')"
claim_challenge="$(canonical_pairing_claim "$qr_payload" "$DEVICE_B" "$PUB_B")"
claim_signature="$(sign_hex "$KEY_B_FILE" "$claim_challenge")"
claim_body="$(jq -cn \
  --arg qr_payload "$qr_payload" \
  --arg new_device_id "$DEVICE_B" \
  --arg new_device_name "Live Smoke Device B" \
  --arg new_device_pubkey "$PUB_B" \
  --arg signature "$claim_signature" \
  '{qr_payload:$qr_payload,new_device_id:$new_device_id,new_device_name:$new_device_name,new_device_pubkey:$new_device_pubkey,signature:$signature}')"
api POST /api/v1/devices/pairing/claim '' "$claim_body" 201 >/dev/null

echo "==> Verify paired Device B"
TOKEN_B="$(verify_device "$DEVICE_B" "$PUB_A" "$KEY_A_FILE")"

echo "==> Device A creates knowledge entry ${ENTRY_ID}"
create_response="$(api POST /api/v1/knowledge/entries "$TOKEN_A" "$(jq -cn --arg entry_id "$ENTRY_ID" '{entry_id:$entry_id,title:"Live Two Device",content_markdown:"# Live Two Device",summary:"live two-device smoke",client_event_id:"live-create-1"}')" 201)"
created_version="$(echo "$create_response" | jq -r '.data.version')"
[[ "$created_version" == "1" ]] || { echo "expected created version 1, got ${created_version}" >&2; exit 1; }

create_pull="$(api GET '/api/v1/sync/events?limit=100' "$TOKEN_B" '' 200)"
assert_event "$create_pull" knowledge.created "$ENTRY_ID"
ack_pull "$TOKEN_B" "$create_pull"

echo "==> Device A updates knowledge entry"
update_response="$(api PUT "/api/v1/knowledge/entries/${ENTRY_ID}" "$TOKEN_A" "$(jq -cn --argjson base_version "$created_version" '{title:"Live Two Device v2",content_markdown:"# Live Two Device v2",summary:"live two-device smoke v2",client_event_id:"live-update-1",base_version:$base_version}')" 200)"
updated_version="$(echo "$update_response" | jq -r '.data.version')"
[[ "$updated_version" == "2" ]] || { echo "expected updated version 2, got ${updated_version}" >&2; exit 1; }

update_pull="$(api GET '/api/v1/sync/events?limit=100' "$TOKEN_B" '' 200)"
assert_event "$update_pull" knowledge.updated "$ENTRY_ID"
ack_pull "$TOKEN_B" "$update_pull"

echo "==> Device B stale update returns 409 and emits no event"
api PUT "/api/v1/knowledge/entries/${ENTRY_ID}" "$TOKEN_B" "$(jq -cn --argjson base_version "$created_version" '{title:"stale",content_markdown:"# stale",summary:"stale",client_event_id:"live-stale-1",base_version:$base_version}')" 409 >/dev/null
empty_after_conflict="$(api GET '/api/v1/sync/events?limit=100' "$TOKEN_A" '' 200)"
assert_event "$empty_after_conflict" '' '' 0

echo "==> Device A deletes knowledge entry"
delete_response="$(api DELETE "/api/v1/knowledge/entries/${ENTRY_ID}" "$TOKEN_A" "$(jq -cn --argjson base_version "$updated_version" '{client_event_id:"live-delete-1",base_version:$base_version}')" 200)"
deleted_version="$(echo "$delete_response" | jq -r '.data.version')"
deleted_status="$(echo "$delete_response" | jq -r '.data.status')"
[[ "$deleted_version" == "3" && "$deleted_status" == "deleted" ]] || { echo "expected deleted v3 tombstone, got version=${deleted_version} status=${deleted_status}" >&2; exit 1; }

delete_pull="$(api GET '/api/v1/sync/events?limit=100' "$TOKEN_B" '' 200)"
assert_event "$delete_pull" knowledge.deleted "$ENTRY_ID"
ack_pull "$TOKEN_B" "$delete_pull"

final_pull="$(api GET '/api/v1/sync/events?limit=100' "$TOKEN_B" '' 200)"
assert_event "$final_pull" '' '' 0

echo
echo "Live two-device knowledge sync smoke passed."
echo "  Device A: ${DEVICE_A}"
echo "  Device B: ${DEVICE_B}"
echo "  Entry:    ${ENTRY_ID}"
