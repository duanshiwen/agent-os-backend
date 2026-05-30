#!/usr/bin/env bash
set -euo pipefail

BASE_URL="${BASE_URL:-http://localhost:8080}"
RUN_ID="${RUN_ID:-$(date +%s)}"
DEVICE_ID="${DEVICE_ID:-object-smoke-device-${RUN_ID}}"
CONTENT="${CONTENT:-AgentOS object storage smoke ${RUN_ID}}"
FILENAME="${FILENAME:-object-smoke-${RUN_ID}.txt}"

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
CONTENT_FILE="$KEY_DIR/$FILENAME"
DOWNLOAD_FILE="$KEY_DIR/downloaded.txt"

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
printf '%s' "$CONTENT" > "$CONTENT_FILE"
SHA256="$(shasum -a 256 "$CONTENT_FILE" | awk '{print $1}')"
SIZE="$(wc -c < "$CONTENT_FILE" | tr -d ' ')"

echo "==> Create upload intent"
intent="$(api POST /api/v1/objects/upload-intents "$TOKEN" "$(jq -cn --arg filename "$FILENAME" --arg sha "$SHA256" --argjson size "$SIZE" '{scope:"smoke/object-storage",filename:$filename,content_type:"text/plain",content_size:$size,sha256:$sha}')" 201)"
object_id="$(echo "$intent" | jq -r '.data.object.id')"
upload_url="$(echo "$intent" | jq -r '.data.upload_url')"
object_uri="$(echo "$intent" | jq -r '.data.object.object_uri')"
if [[ -z "$object_id" || "$object_id" == "null" || -z "$upload_url" || "$upload_url" == "null" ]]; then
  echo "upload intent missing object id or upload url" >&2
  echo "$intent" | jq >&2
  exit 1
fi

echo "==> Upload object to MinIO via presigned PUT"
curl -sS -X PUT -H 'Content-Type: text/plain' --data-binary "@$CONTENT_FILE" "$upload_url" >/dev/null

echo "==> Complete upload"
completed="$(api POST "/api/v1/objects/uploads/${object_id}/complete" "$TOKEN" "$(jq -cn --arg hash "$SHA256" --argjson size "$SIZE" '{observed_hash:$hash,observed_size:$size}')" 200)"
status="$(echo "$completed" | jq -r '.data.status')"
if [[ "$status" != "active" ]]; then
  echo "expected completed object to be active, got ${status}" >&2
  echo "$completed" | jq >&2
  exit 1
fi

echo "==> Create download URL"
download_response="$(api POST "/api/v1/objects/${object_id}/download-url" "$TOKEN" '{"disposition":"inline"}' 200)"
download_url="$(echo "$download_response" | jq -r '.data.download_url')"
if [[ -z "$download_url" || "$download_url" == "null" ]]; then
  echo "download URL missing" >&2
  echo "$download_response" | jq >&2
  exit 1
fi

curl -sS "$download_url" -o "$DOWNLOAD_FILE"
if ! cmp -s "$CONTENT_FILE" "$DOWNLOAD_FILE"; then
  echo "downloaded object content mismatch" >&2
  exit 1
fi

echo "==> Soft delete object"
deleted="$(api DELETE "/api/v1/objects/${object_id}" "$TOKEN" '' 200)"
deleted_status="$(echo "$deleted" | jq -r '.data.status')"
if [[ "$deleted_status" != "deleted" ]]; then
  echo "expected deleted object status, got ${deleted_status}" >&2
  echo "$deleted" | jq >&2
  exit 1
fi

echo
echo "Object storage smoke passed."
echo "  Object ID:  ${object_id}"
echo "  Object URI: ${object_uri}"
echo "  SHA-256:    ${SHA256}"
