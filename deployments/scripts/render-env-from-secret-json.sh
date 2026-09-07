#!/usr/bin/env bash
set -euo pipefail

if [[ $# -ne 2 ]]; then
  echo "usage: $0 <secret-json-file> <env-output-file>" >&2
  exit 2
fi

secret_json_file="$1"
env_output_file="$2"

required_keys=(
  APP_ENV
  HTTP_ADDR
  DATABASE_URL
  SERVICE_TOKEN
  INTERNAL_API_URL
  POSTGRES_PASSWORD
  TELEGRAM_BOT_TOKEN
  TELEGRAM_OWNER_ID
  TELEGRAM_API_ID
  TELEGRAM_API_HASH
  TELEGRAM_PROXY_URL
  TDLIB_DATABASE_DIR
  LLM_PROVIDER
  LLM_API_KEY
  LLM_MODEL
  EXPORT_DIR
)

optional_keys=(
  LOG_FORMAT
  LOG_LEVEL
  LOG_DIR
  LOG_RETENTION
  LOG_ROTATION_INTERVAL
  SUMMARY_RETENTION
  WORKER_ID
  TDLIB_GIT_REF
  LLM_BASE_URL
  LLM_TIMEOUT
  ENCRYPTION_KEY
  OBSIDIAN_REST_URL
  OBSIDIAN_API_KEY
  OBSIDIAN_INSECURE_SKIP_VERIFY
)

for key in "${required_keys[@]}"; do
  if ! jq -e --arg key "$key" 'has($key) and (.[$key] | tostring | length > 0)' "$secret_json_file" >/dev/null; then
    echo "missing required secret key: $key" >&2
    exit 1
  fi
done

tmp_file="$(mktemp)"
trap 'rm -f "$tmp_file"' EXIT

{
  for key in "${required_keys[@]}" "${optional_keys[@]}"; do
    jq -r --arg key "$key" '
      if has($key) and .[$key] != null then
        "\($key)=\(.[$key] | tostring | @sh)"
      else
        empty
      end
    ' "$secret_json_file"
  done
} >"$tmp_file"

install -m 600 "$tmp_file" "$env_output_file"
