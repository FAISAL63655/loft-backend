#!/usr/bin/env bash
set -euo pipefail

# Simple seeding helper for local/dev.
# Usage:
#   SEED_SECRET=dev-secret bash ./scripts/seed.sh
# Optional:
#   BACKEND_URL=http://127.0.0.1:4000 SEED_SECRET=dev-secret bash ./scripts/seed.sh
#   SEED_BODY='{"users_registered":5,"users_verified":3}' SEED_SECRET=dev-secret bash ./scripts/seed.sh
#   bash ./scripts/seed.sh ./scripts/seed-payload.json

if [[ -z "${SEED_SECRET:-}" ]]; then
  echo "[seed] SEED_SECRET env var is required" >&2
  exit 1
fi

BACKEND_URL="${BACKEND_URL:-http://127.0.0.1:4000}"
ENDPOINT="$BACKEND_URL/dev/seed"

CONTENT_TYPE=( -H "Content-Type: application/json" )
SECRET_HEADER=( -H "X-Seed-Secret: ${SEED_SECRET}" )

# Build body
BODY='{}'
if [[ -n "${SEED_BODY:-}" ]]; then
  BODY="${SEED_BODY}"
elif [[ $# -ge 1 && -f "$1" ]]; then
  echo "[seed] Using JSON file: $1"
  curl -sS -X POST "${ENDPOINT}" "${SECRET_HEADER[@]}" "${CONTENT_TYPE[@]}" --data-binary @"$1"
  echo
  exit 0
fi

echo "[seed] POST ${ENDPOINT}"
RESP=$(curl -sS -X POST "${ENDPOINT}" "${SECRET_HEADER[@]}" "${CONTENT_TYPE[@]}" -d "${BODY}")
HTTP_EXIT=$?
if [[ ${HTTP_EXIT} -ne 0 ]]; then
  echo "[seed] curl failed with exit code ${HTTP_EXIT}" >&2
  exit ${HTTP_EXIT}
fi

echo "${RESP}" | jq . 2>/dev/null || echo "${RESP}"
