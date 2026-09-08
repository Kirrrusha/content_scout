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
payload_response="$(mktemp)"
decoded_secret="$(mktemp)"
trap 'rm -f "$token_response" "$secret_response" "$payload_response" "$decoded_secret"' EXIT

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

# api_get <label> <output-file> <url> [extra curl args...]
# Writes the body to output-file. Returns 0 on 2xx, 1 otherwise, echoing the
# response body so API validation errors are visible in the job log.
api_get() {
  local label="$1" out="$2" url="$3"
  shift 3
  local status
  status="$(
    curl -sS -o "$out" -w '%{http_code}' \
      -H "Accept: application/json" \
      -H "Authorization: Bearer $access_token" \
      "$@" \
      "$url"
  )"
  if [[ "$status" != 2* ]]; then
    echo "Cloud.ru $label request failed with HTTP $status" >&2
    echo "--- response body ---" >&2
    head -c 2000 "$out" >&2 || true
    echo >&2
    echo "---------------------" >&2
    return 1
  fi
  return 0
}

secret_id="${CLOUDRU_SECRET_ID:-}"

if [[ -z "$secret_id" ]]; then
  : "${CLOUDRU_SECRET_PROJECT_ID:?CLOUDRU_SECRET_PROJECT_ID is required when CLOUDRU_SECRET_ID is not set}"
  echo "Looking up Cloud.ru secret by name from $api_url/secrets" >&2
  api_get "list-secrets" "$secret_response" "$api_url/secrets" \
    -G \
    --data-urlencode "parent_id=$CLOUDRU_SECRET_PROJECT_ID" \
    --data-urlencode "name=$secret_name"
  secret_id="$(
    jq -r '.id // .secret.id // .secrets[0].id // .items[0].id // empty' "$secret_response"
  )"
  if [[ -z "$secret_id" ]]; then
    echo "Cloud.ru Secret Manager response did not include a secret id" >&2
    exit 1
  fi
fi

# SecretManagerService_AccessSecretVersion. The version is an int16 or "latest";
# there is no Google-style ":access" suffix in this API.
echo "Fetching Cloud.ru secret payload from $api_url/secrets/<redacted>/versions/$secret_version/payload" >&2
api_get "access-secret-version" "$payload_response" \
  "$api_url/secrets/$secret_id/versions/$secret_version/payload"

secret_payload="$(
  jq -r '
    (.data | select(type == "string"))
    // .data.value
    // .payload.data
    // .value
    // empty
  ' "$payload_response"
)"

if [[ -z "$secret_payload" ]]; then
  echo "Cloud.ru Secret Manager response did not include payload data" >&2
  echo "--- response body ---" >&2
  head -c 2000 "$payload_response" >&2 || true
  echo >&2
  exit 1
fi

printf '%s' "$secret_payload" | base64 --decode >"$decoded_secret"
jq -e 'type == "object"' "$decoded_secret" >/dev/null
install -m 600 "$decoded_secret" "$output_file"
