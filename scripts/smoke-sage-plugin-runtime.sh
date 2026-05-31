#!/usr/bin/env bash
set -euo pipefail

BASE_URL="${BASE_URL:-http://localhost:8080}"
if [[ -z "${JWT_SECRET:-}" && -f .env ]]; then
  JWT_SECRET="$(grep -E '^JWT_SECRET=' .env | tail -1 | cut -d= -f2-)"
fi
JWT_SECRET="${JWT_SECRET:-dev-secret-change-me-use-48-plus-bytes-in-production}"
MANIFEST_FILE="${MANIFEST_FILE:-examples/sage-plugins/hotel-booking/sage-plugin.json}"
RUN_ID="${RUN_ID:-$(date +%s)}"
DEVICE_ID="sage-smoke-device-${RUN_ID}"
MOCK_SERVER_PID=""
KEY_DIR=""

cleanup() {
  if [[ -n "${MOCK_SERVER_PID}" ]] && kill -0 "${MOCK_SERVER_PID}" >/dev/null 2>&1; then
    kill "${MOCK_SERVER_PID}" >/dev/null 2>&1 || true
    wait "${MOCK_SERVER_PID}" >/dev/null 2>&1 || true
  fi
  rm -rf "${KEY_DIR:-}"
}
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
  local challenge nonce signature verify_body verify_response token user_id
  challenge="$(api POST /api/v1/auth/challenge '' "$(jq -cn --arg device_id "$device_id" --arg user_pubkey "$pubkey" '{device_id:$device_id,user_pubkey:$user_pubkey}')" 200)"
  nonce="$(echo "$challenge" | jq -r '.data.nonce')"
  signature="$(sign_hex "$key_file" "$(echo "$challenge" | jq -r '.data.challenge')")"
  verify_body="$(jq -cn --arg device_id "$device_id" --arg user_pubkey "$pubkey" --arg nonce "$nonce" --arg signature "$signature" '{device_id:$device_id,user_pubkey:$user_pubkey,nonce:$nonce,signature:$signature}')"
  verify_response="$(api POST /api/v1/auth/verify '' "$verify_body" 200)"
  token="$(echo "$verify_response" | jq -r '.data.access_token')"
  user_id="$(echo "$verify_response" | jq -r '.data.user.id')"
  if [[ -z "$token" || "$token" == "null" || -z "$user_id" || "$user_id" == "null" ]]; then
    echo "auth verify did not return access token and user id" >&2
    echo "$verify_response" >&2
    exit 1
  fi
  printf '%s|%s\n' "$token" "$user_id"
}

make_user() {
  local device_id="$1"
  local identity key pubkey
  identity="$(new_identity "$device_id")"
  key="${identity%%|*}"
  pubkey="${identity##*|}"
  verify_device "$device_id" "$key" "$pubkey"
}

jwt_b64url() {
  openssl base64 -A | tr '+/' '-_' | tr -d '='
}

make_admin_token() {
  local user_id="$1"
  local device_id="$2"
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

start_mock_server() {
  if curl -fsS http://127.0.0.1:18080/sage/health >/dev/null 2>&1; then
    echo "==> Reusing existing mock SAGE Plugin Server on :18080"
    return
  fi
  echo "==> Start mock SAGE Plugin Server"
  python3 examples/sage-plugins/hotel-booking/mock_server.py >/tmp/agentos-sage-plugin-mock-${RUN_ID}.log 2>&1 &
  MOCK_SERVER_PID="$!"
  for _ in $(seq 1 30); do
    if curl -fsS http://127.0.0.1:18080/sage/health >/dev/null 2>&1; then
      return
    fi
    sleep 1
  done
  echo "mock SAGE Plugin Server did not start" >&2
  cat /tmp/agentos-sage-plugin-mock-${RUN_ID}.log >&2 || true
  exit 1
}

echo "==> Health check ${BASE_URL}/health"
api GET /health '' '' 200 | jq '{status, checks, identity_verifier}'

start_mock_server

echo "==> Verify developer identity"
USER_AND_TOKEN="$(make_user "$DEVICE_ID")"
TOKEN="${USER_AND_TOKEN%%|*}"
USER_ID="${USER_AND_TOKEN##*|}"
ADMIN_TOKEN="$(make_admin_token "$USER_ID" "$DEVICE_ID")"

# Smoke-only admin bootstrap. The product still needs a first-class admin bootstrap policy.
echo "==> Mark smoke user as admin in local PostgreSQL"
docker compose exec -T postgres psql -U postgres -d agent_os -v ON_ERROR_STOP=1 -c "UPDATE users SET role='admin', is_admin=true WHERE id='${USER_ID}';" >/dev/null

manifest="$(cat "$MANIFEST_FILE")"
plugin_key="$(echo "$manifest" | jq -r '.plugin_key')"
plugin_name="$(echo "$manifest" | jq -r '.name')"

unique_key="${plugin_key}.smoke.${RUN_ID}"
manifest="$(echo "$manifest" | jq --arg key "$unique_key" '.plugin_key=$key | .endpoints.manifest="http://127.0.0.1:18080/.well-known/sage-plugin.json" | .endpoints.flow="http://127.0.0.1:18080/sage/flow" | .endpoints.callback="http://127.0.0.1:18080/sage/callback" | .endpoints.health="http://127.0.0.1:18080/sage/health"')"

echo "==> Developer creates plugin draft"
plugin_resp="$(api POST /api/v1/sage/plugins "$TOKEN" "$(jq -cn --arg plugin_key "$unique_key" --arg name "$plugin_name" '{plugin_key:$plugin_key,name:$name,description:"SAGE runtime smoke plugin",manifest_url:"http://127.0.0.1:18080/.well-known/sage-plugin.json"}')" 201)"
plugin_id="$(echo "$plugin_resp" | jq -r '.data.id')"

echo "==> Developer submits manifest version"
version_resp="$(api POST "/api/v1/sage/plugins/${plugin_id}/versions" "$TOKEN" "$(jq -n --argjson manifest "$manifest" '{manifest:$manifest}')" 201)"
version_id="$(echo "$version_resp" | jq -r '.data.version.id')"
valid="$(echo "$version_resp" | jq -r '.data.validation.valid')"
if [[ "$valid" != "true" ]]; then
  echo "manifest validation failed" >&2
  echo "$version_resp" | jq >&2
  exit 1
fi

echo "==> Admin approves plugin version"
api POST "/api/v1/admin/sage/plugins/${plugin_id}/versions/${version_id}/review" "$ADMIN_TOKEN" '{"decision":"approved","reason":"runtime contract smoke"}' 200 >/dev/null

echo "==> Public catalog exposes approved plugin"
api GET "/api/v1/sage/catalog/plugins/${unique_key}" '' '' 200 | jq '{id:.data.id, plugin_key:.data.plugin_key, status:.data.status, review_status:.data.review_status}'

echo "==> User installs plugin and grants low-risk permission"
install_resp="$(api POST "/api/v1/sage/catalog/plugins/${unique_key}/install" "$TOKEN" '{}' 201)"
installation_id="$(echo "$install_resp" | jq -r '.data.id')"
grant_resp="$(api POST "/api/v1/sage/installations/${installation_id}/grants" "$TOKEN" '{"permission_key":"plugin.api.call"}' 201)"
grant_id="$(echo "$grant_resp" | jq -r '.data.id')"

echo "==> Sync pull exposes install and grant events"
sync_events="$(api GET '/api/v1/sync/events?after_sequence=0&limit=100' "$TOKEN" '' 200)"
echo "$sync_events" | jq '[.data.events[] | select(.object_type=="plugin") | {event_type, object_id, operation, permission_key:.payload.permission_key}]'
for expected_event in plugin.installed plugin.permission_granted; do
  if [[ "$(echo "$sync_events" | jq --arg event "$expected_event" --arg id "$installation_id" '[.data.events[] | select(.event_type==$event and .object_id==$id)] | length')" == "0" ]]; then
    echo "expected sync event ${expected_event} for installation ${installation_id}" >&2
    echo "$sync_events" | jq >&2
    exit 1
  fi
done

# Exercise the rest of the plugin lifecycle sync contract on the same installation.
api DELETE "/api/v1/sage/installations/${installation_id}/grants/${grant_id}" "$TOKEN" '' 200 >/dev/null
api POST "/api/v1/sage/installations/${installation_id}/disable" "$TOKEN" '{}' 200 >/dev/null
api POST "/api/v1/sage/installations/${installation_id}/enable" "$TOKEN" '{}' 200 >/dev/null
sync_events="$(api GET '/api/v1/sync/events?after_sequence=0&limit=100' "$TOKEN" '' 200)"
for expected_event in plugin.permission_revoked plugin.disabled plugin.enabled; do
  if [[ "$(echo "$sync_events" | jq --arg event "$expected_event" --arg id "$installation_id" '[.data.events[] | select(.event_type==$event and .object_id==$id)] | length')" == "0" ]]; then
    echo "expected sync event ${expected_event} for installation ${installation_id}" >&2
    echo "$sync_events" | jq >&2
    exit 1
  fi
done

# Re-grant the permission so the policy bundle and invocation path remain allowed.
grant_resp="$(api POST "/api/v1/sage/installations/${installation_id}/grants" "$TOKEN" '{"permission_key":"plugin.api.call"}' 201)"
grant_id="$(echo "$grant_resp" | jq -r '.data.id')"

echo "==> Client fetches policy bundle"
bundle="$(api GET "/api/v1/sage/installations/${installation_id}/policy-bundle" "$TOKEN" '' 200)"
echo "$bundle" | jq '{plugin_key:.data.plugin_key, version:.data.version, grants:(.data.granted_permissions|length), guards:(.data.runtime_guards|length)}'
if [[ "$(echo "$bundle" | jq -r '.data.plugin_key')" != "$unique_key" ]]; then
  echo "unexpected policy bundle plugin key" >&2
  echo "$bundle" | jq >&2
  exit 1
fi

flow_url="$(echo "$bundle" | jq -r '.data.manifest_snapshot.endpoints.flow')"

echo "==> Mock AgentOS Client calls Plugin Server flow endpoint"
flow_resp="$(curl -fsS -X POST "$flow_url" -H 'Content-Type: application/json' -d "$(jq -cn --arg intent "find hotels in Hangzhou" --arg installation_id "$installation_id" '{intent:$intent,installation_id:$installation_id,policy_bundle_version:"v1"}')")"
echo "$flow_resp" | jq '{flow_id, steps:(.steps|length)}'
flow_id="$(echo "$flow_resp" | jq -r '.flow_id')"

echo "==> Client creates invocation and submits execution report"
invocation_resp="$(api POST /api/v1/sage/invocations "$TOKEN" "$(jq -cn --arg plugin_key "$unique_key" --arg installation_id "$installation_id" --arg flow_id "$flow_id" '{plugin_key:$plugin_key,installation_id:$installation_id,client_request_id:"sage-runtime-smoke-req",user_intent:"find hotels in Hangzhou",flow_id:$flow_id,flow_hash:"mock-flow-hash",risk_level:"low",permissions_used:["plugin.api.call"],policy_decision:{decision:"allow"}}')" 201)"
invocation_id="$(echo "$invocation_resp" | jq -r '.data.id')"
api POST "/api/v1/sage/invocations/${invocation_id}/reports" "$TOKEN" "$(jq -cn --arg flow_id "$flow_id" '{client_report_id:"sage-runtime-smoke-report",status:"completed",flow_id:$flow_id,steps_completed:["search_hotels","present_options"],step_summaries:{search_hotels:"mock candidates returned",present_options:"options presented"},tokens_used:42,metering:{unit:"tokens"}}')" 201 >/dev/null

echo "==> Uninstall event is pullable after runtime reporting"
api DELETE "/api/v1/sage/installations/${installation_id}" "$TOKEN" '' 200 >/dev/null
sync_events="$(api GET '/api/v1/sync/events?after_sequence=0&limit=100' "$TOKEN" '' 200)"
if [[ "$(echo "$sync_events" | jq --arg id "$installation_id" '[.data.events[] | select(.event_type=="plugin.uninstalled" and .object_id==$id)] | length')" == "0" ]]; then
  echo "expected sync event plugin.uninstalled for installation ${installation_id}" >&2
  echo "$sync_events" | jq >&2
  exit 1
fi

echo "==> Developer metrics include invocation"
metrics="$(api GET "/api/v1/developer/sage/plugins/${plugin_id}/metrics" "$TOKEN" '' 200)"
echo "$metrics" | jq '{plugin_key:.data.plugin_key, invocations:.data.invocations, completed:.data.completed, tokens_used:.data.tokens_used}'
if [[ "$(echo "$metrics" | jq -r '.data.plugin_key')" != "$unique_key" || "$(echo "$metrics" | jq -r '.data.invocations')" == "0" || "$(echo "$metrics" | jq -r '.data.completed')" == "0" ]]; then
  echo "expected developer metrics to include plugin key and completed invocation" >&2
  echo "$metrics" | jq >&2
  exit 1
fi

echo
echo "SAGE plugin runtime/control-plane smoke passed."
echo "  Plugin key:      ${unique_key}"
echo "  Plugin ID:       ${plugin_id}"
echo "  Installation ID: ${installation_id}"
echo "  Invocation ID:   ${invocation_id}"
