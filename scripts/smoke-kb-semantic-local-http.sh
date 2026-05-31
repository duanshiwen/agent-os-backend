#!/usr/bin/env bash
set -euo pipefail

BASE_URL="${BASE_URL:-http://localhost:18081}"
EMBEDDING_ENDPOINT="${EMBEDDING_ENDPOINT:-http://localhost:8091}"
RUN_ID="${RUN_ID:-$(date +%s)}"
PUBLISHER_DEVICE_ID="${PUBLISHER_DEVICE_ID:-kb-semantic-local-publisher-${RUN_ID}}"
ENTRY_ID="notes/kb-semantic-local-${RUN_ID}"
WORKER_PID=""

cleanup() {
  if [[ -n "${WORKER_PID}" ]] && kill -0 "${WORKER_PID}" >/dev/null 2>&1; then
    kill "${WORKER_PID}" >/dev/null 2>&1 || true
    wait "${WORKER_PID}" >/dev/null 2>&1 || true
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

wait_for_coverage() {
  local collection_id="$1"
  local snapshot_id="$2"
  local token="$3"
  local status coverage ready pending failed
  for _ in $(seq 1 60); do
    status="$(api GET "/api/v1/kb/collections/${collection_id}/snapshots/${snapshot_id}/embedding-status" "$token" '' 200)"
    coverage="$(echo "$status" | jq -r '.data.coverage.coverage // 0')"
    ready="$(echo "$status" | jq -r '.data.coverage.ready_embeddings // 0')"
    pending="$(echo "$status" | jq -r '.data.coverage.pending_jobs // 0')"
    failed="$(echo "$status" | jq -r '.data.coverage.failed_jobs // 0')"
    echo "coverage=${coverage} ready=${ready} pending=${pending} failed=${failed}"
    if [[ "$coverage" == "1" || "$coverage" == "1.0" ]]; then
      return 0
    fi
    sleep 2
  done
  echo "embedding coverage did not reach 1.0" >&2
  api GET "/api/v1/kb/collections/${collection_id}/snapshots/${snapshot_id}/embedding-status" "$token" '' 200 | jq >&2
  echo "worker log: /tmp/agentos-semantic-local-worker-${RUN_ID}.log" >&2
  exit 1
}

wait_for_worker() {
  echo "==> Check real embedding worker metadata ${EMBEDDING_ENDPOINT}/metadata"
  curl -fsS "${EMBEDDING_ENDPOINT}/metadata" | jq '{provider, model, dimensions, normalized}'
}

wait_for_worker

echo "==> Health check ${BASE_URL}/health"
api GET /health '' '' 200 | jq '{status, checks, identity_verifier}'

echo "==> Verify publisher identity"
PUBLISHER_TOKEN="$(make_token "$PUBLISHER_DEVICE_ID")"

echo "==> Create knowledge entry, collection, and snapshot"
api POST /api/v1/knowledge/entries "$PUBLISHER_TOKEN" "$(jq -cn --arg entry_id "$ENTRY_ID" '{entry_id:$entry_id,title:"真实语义搜索：蓝海战略",content_markdown:"# 蓝海战略\n\n通过价值创新创造无人竞争的市场空间，并同时追求差异化与低成本。",summary:"这是一条用于真实 BGE-M3 语义检索 smoke 的中文知识条目，主题是蓝海战略、价值创新和无人竞争市场。",tags:["semantic","strategy","blue-ocean","中文"],metadata:{smoke:true,source:"local-http-bge-m3"},client_event_id:"kb-semantic-local-create-1"}')" 201 >/dev/null
collection_response="$(api POST /api/v1/kb/collections "$PUBLISHER_TOKEN" "$(jq -cn --arg name "Semantic Local KB ${RUN_ID}" '{name:$name,description:"local_http BGE-M3 semantic smoke"}')" 201)"
collection_id="$(echo "$collection_response" | jq -r '.data.id')"
snapshot_response="$(api POST "/api/v1/kb/collections/${collection_id}/snapshots" "$PUBLISHER_TOKEN" "$(jq -cn --arg entry_id "$ENTRY_ID" '{entry_ids:[$entry_id]}')" 201)"
snapshot_id="$(echo "$snapshot_response" | jq -r '.data.snapshot.id')"

echo "==> Start local_http embedding job worker"
EMBEDDING_PROVIDER=local_http \
EMBEDDING_ENDPOINT="$EMBEDDING_ENDPOINT" \
EMBEDDING_MODEL=BAAI/bge-m3 \
EMBEDDING_DIMENSIONS=1024 \
go run ./cmd/worker >/tmp/agentos-semantic-local-worker-${RUN_ID}.log 2>&1 &
WORKER_PID="$!"

wait_for_coverage "$collection_id" "$snapshot_id" "$PUBLISHER_TOKEN"

echo "==> Semantic search returns the published entry"
semantic_response="$(api GET "/api/v1/kb/public/search?q=%E4%BB%B7%E5%80%BC%E5%88%9B%E6%96%B0%20%E6%97%A0%E4%BA%BA%E7%AB%9E%E4%BA%89%E5%B8%82%E5%9C%BA&mode=semantic&collection_id=${collection_id}&snapshot_id=${snapshot_id}" '' '' 200)"
if [[ "$(echo "$semantic_response" | jq -r '.data.semantic_available')" != "true" ]]; then
  echo "semantic search was not available" >&2
  echo "$semantic_response" | jq >&2
  exit 1
fi
if [[ "$(echo "$semantic_response" | jq -r '.data.items[0].entry_id')" != "$ENTRY_ID" ]]; then
  echo "semantic search did not return expected entry" >&2
  echo "$semantic_response" | jq >&2
  exit 1
fi

echo
printf 'KB semantic local_http smoke passed.\n'
printf '  Collection ID: %s\n' "$collection_id"
printf '  Snapshot ID:   %s\n' "$snapshot_id"
printf '  Entry ID:      %s\n' "$ENTRY_ID"
