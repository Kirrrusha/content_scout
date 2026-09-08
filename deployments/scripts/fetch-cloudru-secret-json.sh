#!/usr/bin/env bash
set -euo pipefail

if [[ $# -ne 1 ]]; then
  echo "usage: $0 <json-output-file>" >&2
  exit 2
fi

output_file="$1"

: "${CLOUDRU_SECRET_KEY_ID:?CLOUDRU_SECRET_KEY_ID is required}"
: "${CLOUDRU_SECRET_KEY_SECRET:?CLOUDRU_SECRET_KEY_SECRET is required}"

if ! command -v jq >/dev/null 2>&1; then
  echo "jq is required" >&2
  exit 1
fi

secret_name="${CLOUDRU_SECRET_NAME:-content-scout-prod-env}"
secret_version="${CLOUDRU_SECRET_VERSION:-latest}"
auth_url="${CLOUDRU_AUTH_URL:-https://iam.api.cloud.ru/api/v1/auth/token}"
api_url="${CLOUDRU_SECRET_MANAGER_API_URL:-https://secretmanager.api.cloud.ru/v1}"

token_response="$(mktemp)"
secret_response="$(mktemp)"
decoded_secret="$(mktemp)"
trap 'rm -f "$token_response" "$secret_response" "$decoded_secret"' EXIT

auth_payload="$(
  jq -n \
    --arg key_id "$CLOUDRU_SECRET_KEY_ID" \
    --arg secret "$CLOUDRU_SECRET_KEY_SECRET" \
    '{keyId: $key_id, secret: $secret}'
)"

echo "Requesting Cloud.ru IAM token from $auth_url" >&2
curl -fsS \
  -H 'Accept: application/json' \
  -H 'Content-Type: application/json' \
  --data "$auth_payload" \
  "$auth_url" >"$token_response"

access_token="$(jq -r '.access_token // empty' "$token_response")"
if [[ -z "$access_token" ]]; then
  echo "Cloud.ru auth response did not include access_token" >&2
  exit 1
fi

if [[ -n "${CLOUDRU_SECRET_ID:-}" ]]; then
  echo "Fetching Cloud.ru secret by id from $api_url/secrets/<redacted>" >&2
  curl -fsS \
    -H "Accept: application/json" \
    -H "Content-Type: application/json" \
    -H "Authorization: Bearer $access_token" \
    "$api_url/secrets/$CLOUDRU_SECRET_ID" >"$secret_response"
else
  : "${CLOUDRU_SECRET_PROJECT_ID:?CLOUDRU_SECRET_PROJECT_ID is required when CLOUDRU_SECRET_ID is not set}"
  echo "Fetching Cloud.ru secret by name from $api_url/secrets" >&2
  curl -fsS \
    -G \
    -H "Accept: application/json" \
    -H "Content-Type: application/json" \
    -H "Authorization: Bearer $access_token" \
    --data-urlencode "parent_id=$CLOUDRU_SECRET_PROJECT_ID" \
    --data-urlencode "name=$secret_name" \
    "$api_url/secrets" >"$secret_response"
fi

extract_payload() {
  jq -r '
    .payload.data.value
    // .payload.data
    // .secret.payload.data.value
    // .secret.payload.data
    // .secret_version.payload.data.value
    // .secret_version.payload.data
    // .version.payload.data.value
    // .version.payload.data
    // .secrets[0].payload.data.value
    // .secrets[0].payload.data
    // .items[0].payload.data.value
    // .items[0].payload.data
    // .data.value
    // .value
    // empty
  ' "$1"
}

secret_payload="$(extract_payload "$secret_response")"

if [[ -z "$secret_payload" ]]; then
  resolved_secret_id="${CLOUDRU_SECRET_ID:-}"
  if [[ -z "$resolved_secret_id" ]]; then
    resolved_secret_id="$(
      jq -r '
        .id
        // .secret.id
        // .secrets[0].id
        // .items[0].id
        // empty
      ' "$secret_response"
    )"
  fi

  if [[ -z "$resolved_secret_id" ]]; then
    echo "Cloud.ru Secret Manager response did not include secret id or payload data" >&2
    exit 1
  fi

  echo "Fetching Cloud.ru secret version $secret_version from $api_url/secrets/<redacted>/versions/<redacted>" >&2
  if ! curl -fsS \
    -H "Accept: application/json" \
    -H "Content-Type: application/json" \
    -H "Authorization: Bearer $access_token" \
    "$api_url/secrets/$resolved_secret_id/versions/$secret_version" >"$secret_response"; then
    echo "Retrying Cloud.ru secret version with access endpoint" >&2
    curl -fsS \
      -H "Accept: application/json" \
      -H "Content-Type: application/json" \
      -H "Authorization: Bearer $access_token" \
      "$api_url/secrets/$resolved_secret_id/versions/$secret_version:access" >"$secret_response"
  fi

  secret_payload="$(extract_payload "$secret_response")"
fi

if [[ -z "$secret_payload" ]]; then
  echo "Cloud.ru Secret Manager response did not include payload data" >&2
  exit 1
fi

printf '%s' "$secret_payload" | base64 --decode >"$decoded_secret"
jq -e 'type == "object"' "$decoded_secret" >/dev/null
install -m 600 "$decoded_secret" "$output_file"
