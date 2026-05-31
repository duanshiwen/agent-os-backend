#!/usr/bin/env bash
set -euo pipefail

BASE_URL="${BASE_URL:-http://localhost:8080}"
if [[ -z "${JWT_SECRET:-}" && -f .env ]]; then
  JWT_SECRET="$(grep -E '^JWT_SECRET=' .env | tail -1 | cut -d= -f2-)"
fi
JWT_SECRET="${JWT_SECRET:-dev-secret-change-me-use-48-plus-bytes-in-production}"
RUN_ID="${RUN_ID:-$(date +%s)}"
DEVICE_ID="governance-smoke-device-${RUN_ID}"
KEY_DIR=""

cleanup() { rm -rf "${KEY_DIR:-}"; }
trap cleanup EXIT

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
require_tool python3

api() {
  local method="$1" path="$2" token="${3:-}" body="${4:-}" expected_status="${5:-}" response_file status
  response_file="$(mktemp)"
  if [[ -n "$body" ]]; then
    status="$(curl -sS -o "$response_file" -w '%{http_code}' -X "$method" -H 'Content-Type: application/json' ${token:+-H "Authorization: Bearer ${token}"} -d "$body" "${BASE_URL}${path}")"
  else
    status="$(curl -sS -o "$response_file" -w '%{http_code}' -X "$method" ${token:+-H "Authorization: Bearer ${token}"} "${BASE_URL}${path}")"
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

new_identity() {
  local device_id="$1" key_file="$KEY_DIR/${device_id}.pem" pubkey
  openssl genpkey -algorithm Ed25519 -out "$key_file" >/dev/null 2>&1
  pubkey="$(openssl pkey -in "$key_file" -pubout -outform DER 2>/dev/null | xxd -p -c 256 | sed 's/^.*032100//')"
  echo "$key_file|$pubkey"
}

sign_hex() {
  local key_file="$1" message="$2" message_file signature_file
  message_file="$(mktemp "$KEY_DIR/message.XXXXXX")"
  signature_file="$(mktemp "$KEY_DIR/signature.XXXXXX")"
  printf '%s' "$message" > "$message_file"
  openssl pkeyutl -sign -rawin -inkey "$key_file" -in "$message_file" -out "$signature_file"
  xxd -p -c 256 "$signature_file"
}

make_user() {
  local identity key pubkey challenge nonce signature verify_body verify_response token user_id
  identity="$(new_identity "$DEVICE_ID")"
  key="${identity%%|*}"
  pubkey="${identity##*|}"
  challenge="$(api POST /api/v1/auth/challenge '' "$(jq -cn --arg device_id "$DEVICE_ID" --arg user_pubkey "$pubkey" '{device_id:$device_id,user_pubkey:$user_pubkey}')" 200)"
  nonce="$(echo "$challenge" | jq -r '.data.nonce')"
  signature="$(sign_hex "$key" "$(echo "$challenge" | jq -r '.data.challenge')")"
  verify_body="$(jq -cn --arg device_id "$DEVICE_ID" --arg user_pubkey "$pubkey" --arg nonce "$nonce" --arg signature "$signature" '{device_id:$device_id,user_pubkey:$user_pubkey,nonce:$nonce,signature:$signature}')"
  verify_response="$(api POST /api/v1/auth/verify '' "$verify_body" 200)"
  token="$(echo "$verify_response" | jq -r '.data.access_token')"
  user_id="$(echo "$verify_response" | jq -r '.data.user.id')"
  printf '%s|%s\n' "$token" "$user_id"
}

make_admin_token() {
  local user_id="$1" device_id="$2"
  python3 - "$JWT_SECRET" "$user_id" "$device_id" <<'PY'
import base64, hashlib, hmac, json, sys, time
secret, user_id, device_id = sys.argv[1:]
def b64(data: bytes) -> str:
    return base64.urlsafe_b64encode(data).decode().rstrip('=')
header = {"alg": "HS256", "typ": "JWT"}
now = int(time.time())
payload = {"iss": "agent-os", "iat": now, "exp": now + 3600, "user_id": user_id, "device_id": device_id}
msg = b64(json.dumps(header, separators=(',', ':')).encode()) + '.' + b64(json.dumps(payload, separators=(',', ':')).encode())
sig = hmac.new(secret.encode(), msg.encode(), hashlib.sha256).digest()
print(msg + '.' + b64(sig))
PY
}

USER_AUTH="$(make_user)"
USER_ID="${USER_AUTH##*|}"
ADMIN_TOKEN="$(make_admin_token "$USER_ID" "$DEVICE_ID")"

api POST /api/v1/admin/governance/capabilities "$ADMIN_TOKEN" "$(jq -cn '{key:"sage.permission.payments.write",name:"SAGE payments write",risk_level:"high",status:"active"}')" 201 >/dev/null
api POST /api/v1/admin/governance/policy-rules "$ADMIN_TOKEN" "$(jq -cn '{name:"payments require user approval",capability_key:"sage.permission.payments.write",subject_type:"sage_permission_grant",effect:"require_user_approval",priority:1,status:"active"}')" 201 >/dev/null
DECISION_JSON="$(api POST /api/v1/admin/governance/evaluate "$ADMIN_TOKEN" "$(jq -cn '{subject_type:"sage_permission_grant",subject_id:"smoke-grant",capability_key:"sage.permission.payments.write",risk_level:"high"}')" 200)"
DECISION="$(echo "$DECISION_JSON" | jq -r '.data.decision')"
if [[ "$DECISION" != "require_user_approval" ]]; then
  echo "expected require_user_approval decision, got ${DECISION}" >&2
  echo "$DECISION_JSON" | jq >&2
  exit 1
fi
VERIFY_JSON="$(api GET /api/v1/admin/audit/verify-chain "$ADMIN_TOKEN" '' 200)"
VALID="$(echo "$VERIFY_JSON" | jq -r '.data.valid')"
if [[ "$VALID" != "true" ]]; then
  echo "expected audit hash chain valid after governance operations" >&2
  echo "$VERIFY_JSON" | jq >&2
  exit 1
fi

echo "Governance enforcement smoke passed. Decision=${DECISION} audit_chain_valid=${VALID}"
