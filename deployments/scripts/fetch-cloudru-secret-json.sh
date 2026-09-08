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
versions_response="$(mktemp)"
decoded_secret="$(mktemp)"
trap 'rm -f "$token_response" "$secret_response" "$versions_response" "$decoded_secret"' EXIT

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
# Writes body to output-file. Returns 0 on 2xx, 1 otherwise (body echoed to stderr).
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
  ' "$1" 2>/dev/null
}

resolved_secret_id="${CLOUDRU_SECRET_ID:-}"

if [[ -n "$resolved_secret_id" ]]; then
  echo "Fetching Cloud.ru secret by id from $api_url/secrets/<redacted>" >&2
  api_get "get-secret" "$secret_response" "$api_url/secrets/$resolved_secret_id"
else
  : "${CLOUDRU_SECRET_PROJECT_ID:?CLOUDRU_SECRET_PROJECT_ID is required when CLOUDRU_SECRET_ID is not set}"
  echo "Fetching Cloud.ru secret by name from $api_url/secrets" >&2
  api_get "list-secrets" "$secret_response" "$api_url/secrets" \
    -G \
    --data-urlencode "parent_id=$CLOUDRU_SECRET_PROJECT_ID" \
    --data-urlencode "name=$secret_name"
  resolved_secret_id="$(
    jq -r '.id // .secret.id // .secrets[0].id // .items[0].id // empty' "$secret_response"
  )"
fi

secret_payload="$(extract_payload "$secret_response")"

if [[ -z "$secret_payload" ]]; then
  if [[ -z "$resolved_secret_id" ]]; then
    echo "Cloud.ru Secret Manager response did not include secret id or payload data" >&2
    exit 1
  fi

  # Resolve a concrete version id: "latest" is not a valid alias in Cloud.ru SM.
  version_id="$secret_version"
  if [[ "$version_id" == "latest" ]]; then
    echo "Listing Cloud.ru secret versions from $api_url/secrets/<redacted>/versions" >&2
    if api_get "list-versions" "$versions_response" "$api_url/secrets/$resolved_secret_id/versions"; then
      version_id="$(
        jq -r '
          (.versions // .items // .secret_versions // [])
          | map(select((.state // .status // "ENABLED") | ascii_upcase | test("ENABLE|ACTIVE")))
          | sort_by(.created_at // .createdAt // .create_time // "")
          | last
          | (.id // .version_id // .versionId // .name // empty)
        ' "$versions_response"
      )"
      # name may come back as "secrets/<id>/versions/<version>"
      version_id="${version_id##*/}"
    fi
    if [[ -z "$version_id" ]]; then
      echo "Could not resolve a concrete Cloud.ru secret version id" >&2
      exit 1
    fi
  fi

  echo "Fetching Cloud.ru secret version payload from access endpoint" >&2
  if ! api_get "access-version" "$secret_response" \
    "$api_url/secrets/$resolved_secret_id/versions/$version_id:access"; then
    echo "Retrying without the :access suffix" >&2
    api_get "get-version" "$secret_response" \
      "$api_url/secrets/$resolved_secret_id/versions/$version_id"
  fi

  secret_payload="$(extract_payload "$secret_response")"
fi

if [[ -z "$secret_payload" ]]; then
  echo "Cloud.ru Secret Manager response did not include payload data" >&2
  echo "--- last response body ---" >&2
  head -c 2000 "$secret_response" >&2 || true
  echo >&2
  exit 1
fi

printf '%s' "$secret_payload" | base64 --decode >"$decoded_secret"
jq -e 'type == "object"' "$decoded_secret" >/dev/null
install -m 600 "$decoded_secret" "$output_file"
