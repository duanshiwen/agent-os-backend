#!/usr/bin/env bash
set -euo pipefail

BASE_URL="${BASE_URL:-http://localhost:8080}"
TOKEN="${TOKEN:-}"
ADMIN_TOKEN="${ADMIN_TOKEN:-$TOKEN}"
MANIFEST_FILE="${MANIFEST_FILE:-examples/sage-plugins/hotel-booking/sage-plugin.json}"

if [[ -z "$TOKEN" ]]; then
  echo "TOKEN is required. Export a user JWT before running." >&2
  exit 2
fi

post_json() {
  local path="$1"
  local body="$2"
  curl -fsS -H "Authorization: Bearer $TOKEN" -H "Content-Type: application/json" -d "$body" "$BASE_URL$path"
}

admin_post_json() {
  local path="$1"
  local body="$2"
  curl -fsS -H "Authorization: Bearer $ADMIN_TOKEN" -H "Content-Type: application/json" -d "$body" "$BASE_URL$path"
}

echo "SAGE smoke requires jq and a running backend."
command -v jq >/dev/null

manifest="$(cat "$MANIFEST_FILE")"
plugin_resp="$(post_json /api/v1/sage/plugins '{"plugin_key":"com.example.hotel-booking","name":"Hotel Booking Assistant","description":"Smoke plugin","manifest_url":"http://localhost:18080/.well-known/sage-plugin.json"}')"
plugin_id="$(echo "$plugin_resp" | jq -r '.data.id')"

version_resp="$(post_json "/api/v1/sage/plugins/$plugin_id/versions" "$(jq -n --argjson manifest "$manifest" '{manifest:$manifest}')")"
version_id="$(echo "$version_resp" | jq -r '.data.version.id')"

admin_post_json "/api/v1/admin/sage/plugins/$plugin_id/versions/$version_id/review" '{"decision":"approved","reason":"smoke"}' >/dev/null

install_resp="$(post_json /api/v1/sage/catalog/plugins/com.example.hotel-booking/install '{}')"
installation_id="$(echo "$install_resp" | jq -r '.data.id')"

post_json "/api/v1/sage/installations/$installation_id/grants" '{"permission_key":"plugin.api.call"}' >/dev/null
curl -fsS -H "Authorization: Bearer $TOKEN" "$BASE_URL/api/v1/sage/installations/$installation_id/policy-bundle" >/dev/null

invocation_resp="$(post_json /api/v1/sage/invocations '{"plugin_key":"com.example.hotel-booking","client_request_id":"smoke-req-1","user_intent":"find hotels"}')"
invocation_id="$(echo "$invocation_resp" | jq -r '.data.id')"
post_json "/api/v1/sage/invocations/$invocation_id/reports" '{"client_report_id":"smoke-report-1","status":"completed","flow_id":"flow_hotel_search_demo","steps_completed":["search_hotels","present_options"],"tokens_used":42}' >/dev/null

echo "SAGE plugin runtime/control-plane smoke completed."
