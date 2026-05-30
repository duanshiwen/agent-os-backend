#!/usr/bin/env bash
set -euo pipefail

BASE_URL="${EMBEDDING_WORKER_URL:-http://localhost:8091}"

echo "[embedding-worker] checking metadata at ${BASE_URL}/metadata"
metadata="$(curl -fsS "${BASE_URL}/metadata")"
echo "${metadata}"

echo "[embedding-worker] checking embed endpoint"
response="$(curl -fsS -X POST "${BASE_URL}/embed" \
  -H 'Content-Type: application/json' \
  -d '{"texts":["Title: Blue Ocean Strategy\n\nSummary: Create uncontested market space"],"normalize":true}')"
python3 -c 'import json, sys
payload = json.load(sys.stdin)
assert payload["model"] == "BAAI/bge-m3", payload
assert payload["dimensions"] == 1024, payload
assert len(payload["vectors"]) == 1, payload
assert len(payload["vectors"][0]) == 1024, len(payload["vectors"][0])
print("[embedding-worker] ok")' <<< "${response}"
