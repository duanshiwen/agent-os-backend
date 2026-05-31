#!/usr/bin/env bash
set -euo pipefail

BASE_URL="${BASE_URL:-http://localhost:8080}"
if [[ -z "${JWT_SECRET:-}" && -f .env ]]; then
  JWT_SECRET="$(grep -E '^JWT_SECRET=' .env | tail -1 | cut -d= -f2-)"
fi
JWT_SECRET="${JWT_SECRET:-dev-secret-change-me-use-48-plus-bytes-in-production}"
RUN_ID="${RUN_ID:-$(date +%s)}"
DEVICE_ID="governance-enforce-smoke-device-${RUN_ID}"
PG_DB_NAME="${PG_DB:-agent_os}"
PG_USER_NAME="${PG_USER:-postgres}"
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
  local device_id key_file pubkey
  device_id="$1"
  key_file="$KEY_DIR/${device_id}.pem"
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

echo "==> Health check ${BASE_URL}/health"
api GET /health '' '' 200 | jq '{status, checks}'

USER_AUTH="$(make_user)"
USER_TOKEN="${USER_AUTH%%|*}"
USER_ID="${USER_AUTH##*|}"
ADMIN_TOKEN="$(make_admin_token "$USER_ID" "$DEVICE_ID")"

# Smoke-only admin bootstrap. The product still needs a first-class admin bootstrap policy.
if command -v docker >/dev/null 2>&1 && docker compose ps postgres >/dev/null 2>&1; then
  echo "==> Mark smoke user as admin in local PostgreSQL (${PG_DB_NAME})"
  docker compose exec -T postgres psql -U "$PG_USER_NAME" -d "$PG_DB_NAME" -v ON_ERROR_STOP=1 -c "UPDATE users SET role='admin', is_admin=true WHERE id='${USER_ID}';" >/dev/null
else
  echo "==> Skipping DB admin bootstrap; assuming supplied token is already admin-capable"
fi

SCOPE="governance-enforce-${RUN_ID}"
CAPABILITY="object.upload.${SCOPE//-/_}"
SHA="$(printf 'governance enforce smoke' | shasum -a 256 | awk '{print $1}')"

# Duplicate capability/rule creates are treated as smoke setup best-effort because repeated RUN_ID values may collide.
api POST /api/v1/admin/governance/capabilities "$ADMIN_TOKEN" "$(jq -cn --arg key "$CAPABILITY" '{key:$key,name:"Object upload enforce smoke",risk_level:"medium",status:"active"}')" '' >/dev/null || true
api POST /api/v1/admin/governance/policy-rules "$ADMIN_TOKEN" "$(jq -cn --arg key "$CAPABILITY" '{name:"object upload enforce smoke deny",capability_key:$key,subject_type:"object_operation",risk_level:"medium",effect:"deny",priority:1,status:"active"}')" '' >/dev/null || true

echo "==> Protected object upload is blocked by governance enforce mode"
BLOCKED_JSON="$(api POST /api/v1/objects/upload-intents "$USER_TOKEN" "$(jq -cn --arg scope "$SCOPE" --arg sha "$SHA" '{scope:$scope,filename:"blocked.txt",content_type:"text/plain",content_size:24,sha256:$sha}')" 403)"
ERROR_CODE="$(echo "$BLOCKED_JSON" | jq -r '.error.code')"
if [[ "$ERROR_CODE" != "governance_denied" ]]; then
  echo "expected stable governance_denied error code, got ${ERROR_CODE}" >&2
  echo "$BLOCKED_JSON" | jq >&2
  exit 1
fi

echo "==> Admin governance read APIs expose the decision evidence"
DECISIONS_JSON="$(api GET "/api/v1/admin/governance/policy-decisions?subject_type=object_operation&capability_key=${CAPABILITY}&decision=deny" "$ADMIN_TOKEN" '' 200)"
DECISION_TOTAL="$(echo "$DECISIONS_JSON" | jq -r '.data.total')"
if [[ "$DECISION_TOTAL" == "0" ]]; then
  echo "expected at least one denied policy decision" >&2
  echo "$DECISIONS_JSON" | jq >&2
  exit 1
fi

SUMMARY_JSON="$(api GET /api/v1/admin/governance/summary?window=24 "$ADMIN_TOKEN" '' 200)"
POLICY_TOTAL="$(echo "$SUMMARY_JSON" | jq -r '.data.policy_decision_total')"
if [[ "$POLICY_TOTAL" == "0" ]]; then
  echo "expected governance summary to include policy decisions" >&2
  echo "$SUMMARY_JSON" | jq >&2
  exit 1
fi

APPROVAL_CAPABILITY="sage.permission.governance_smoke_${RUN_ID}"
APPROVAL_SUBJECT="approval-grant-${RUN_ID}"
api POST /api/v1/admin/governance/capabilities "$ADMIN_TOKEN" "$(jq -cn --arg key "$APPROVAL_CAPABILITY" '{key:$key,name:"Approval workflow smoke",risk_level:"high",status:"active"}')" '' >/dev/null || true
api POST /api/v1/admin/governance/policy-rules "$ADMIN_TOKEN" "$(jq -cn --arg key "$APPROVAL_CAPABILITY" '{name:"approval workflow smoke require approval",capability_key:$key,subject_type:"sage_permission_grant",risk_level:"high",effect:"require_user_approval",priority:1,status:"active"}')" '' >/dev/null || true

EVAL_JSON="$(api POST /api/v1/admin/governance/evaluate "$ADMIN_TOKEN" "$(jq -cn --arg actor "$USER_ID" --arg subject "$APPROVAL_SUBJECT" --arg key "$APPROVAL_CAPABILITY" '{actor_user_id:$actor,subject_type:"sage_permission_grant",subject_id:$subject,capability_key:$key,risk_level:"high"}')" 200)"
APPROVAL_DECISION_ID="$(echo "$EVAL_JSON" | jq -r '.data.id')"
APPROVAL_DECISION="$(echo "$EVAL_JSON" | jq -r '.data.decision')"
if [[ "$APPROVAL_DECISION" != "require_user_approval" ]]; then
  echo "expected require_user_approval decision, got ${APPROVAL_DECISION}" >&2
  echo "$EVAL_JSON" | jq >&2
  exit 1
fi

CREATE_RECEIPT_JSON="$(api POST /api/v1/admin/governance/approval-receipts "$ADMIN_TOKEN" "$(jq -cn --arg decision_id "$APPROVAL_DECISION_ID" --arg actor "$USER_ID" --arg subject "$APPROVAL_SUBJECT" --arg key "$APPROVAL_CAPABILITY" '{policy_decision_id:$decision_id,actor_user_id:$actor,subject_type:"sage_permission_grant",subject_id:$subject,capability_key:$key,decision:"require_user_approval",expires_in_seconds:900}')" 201)"
RECEIPT_ID="$(echo "$CREATE_RECEIPT_JSON" | jq -r '.data.receipt.id')"
OLD_APPROVAL_TOKEN="$(echo "$CREATE_RECEIPT_JSON" | jq -r '.data.approval_token')"

REISSUE_JSON="$(api POST "/api/v1/admin/governance/approval-receipts/${RECEIPT_ID}/reissue-token" "$ADMIN_TOKEN" '{"reason":"smoke operator approved","expires_in_seconds":600}' 200)"
NEW_APPROVAL_TOKEN="$(echo "$REISSUE_JSON" | jq -r '.data.approval_token')"
if [[ -z "$NEW_APPROVAL_TOKEN" || "$NEW_APPROVAL_TOKEN" == "null" || "$NEW_APPROVAL_TOKEN" == "$OLD_APPROVAL_TOKEN" ]]; then
  echo "expected reissued approval token to be present and rotated" >&2
  echo "$REISSUE_JSON" | jq >&2
  exit 1
fi

api POST /api/v1/governance/approval-receipts/consume '' "$(jq -cn --arg token "$OLD_APPROVAL_TOKEN" '{approval_token:$token,consumed_by:"smoke-old-token"}')" 400 >/dev/null
api POST /api/v1/governance/approval-receipts/consume '' "$(jq -cn --arg token "$NEW_APPROVAL_TOKEN" '{approval_token:$token,consumed_by:"smoke-new-token"}')" 200 >/dev/null
api POST "/api/v1/admin/governance/approval-receipts/${RECEIPT_ID}/reissue-token" "$ADMIN_TOKEN" '{"reason":"after consume"}' 400 >/dev/null

REVOKE_SUBJECT="approval-revoke-${RUN_ID}"
REVOKE_EVAL_JSON="$(api POST /api/v1/admin/governance/evaluate "$ADMIN_TOKEN" "$(jq -cn --arg actor "$USER_ID" --arg subject "$REVOKE_SUBJECT" --arg key "$APPROVAL_CAPABILITY" '{actor_user_id:$actor,subject_type:"sage_permission_grant",subject_id:$subject,capability_key:$key,risk_level:"high"}')" 200)"
REVOKE_DECISION_ID="$(echo "$REVOKE_EVAL_JSON" | jq -r '.data.id')"
REVOKE_CREATE_JSON="$(api POST /api/v1/admin/governance/approval-receipts "$ADMIN_TOKEN" "$(jq -cn --arg decision_id "$REVOKE_DECISION_ID" --arg actor "$USER_ID" --arg subject "$REVOKE_SUBJECT" --arg key "$APPROVAL_CAPABILITY" '{policy_decision_id:$decision_id,actor_user_id:$actor,subject_type:"sage_permission_grant",subject_id:$subject,capability_key:$key,decision:"require_user_approval",expires_in_seconds:900}')" 201)"
REVOKE_RECEIPT_ID="$(echo "$REVOKE_CREATE_JSON" | jq -r '.data.receipt.id')"
REVOKE_JSON="$(api POST "/api/v1/admin/governance/approval-receipts/${REVOKE_RECEIPT_ID}/revoke" "$ADMIN_TOKEN" '{"reason":"smoke revoke"}' 200)"
REVOKE_STATUS="$(echo "$REVOKE_JSON" | jq -r '.data.status')"
if [[ "$REVOKE_STATUS" != "revoked" ]]; then
  echo "expected revoked approval receipt, got ${REVOKE_STATUS}" >&2
  echo "$REVOKE_JSON" | jq >&2
  exit 1
fi
api POST "/api/v1/admin/governance/approval-receipts/${REVOKE_RECEIPT_ID}/reissue-token" "$ADMIN_TOKEN" '{"reason":"after revoke"}' 400 >/dev/null

echo "Governance enforce-readiness smoke passed. error_code=${ERROR_CODE} denied_decisions=${DECISION_TOTAL} summary_policy_total=${POLICY_TOTAL} approval_reissue=ok approval_revoke=ok"
