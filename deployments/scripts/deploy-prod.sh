#!/usr/bin/env bash
set -euo pipefail

release_dir="${CONTENT_SCOUT_RELEASE_DIR:-/opt/content_scout}"
compose_file="$release_dir/docker-compose.prod.yml"
secret_json_file="$release_dir/.env.secret.json"
env_file="$release_dir/.env"

: "${CLOUDRU_REGISTRY:?CLOUDRU_REGISTRY is required}"
: "${IMAGE_TAG:?IMAGE_TAG is required}"

mkdir -p "$release_dir"
cd "$release_dir"

if [[ ! -f "$compose_file" ]]; then
  echo "missing compose file: $compose_file" >&2
  exit 1
fi

if [[ ! -f "$secret_json_file" ]]; then
  echo "missing secret json file: $secret_json_file" >&2
  exit 1
fi

./render-env-from-secret-json.sh "$secret_json_file" "$env_file"
rm -f "$secret_json_file"

export CONTENT_SCOUT_ENV_FILE="$env_file"

compose() {
  docker compose --env-file "$env_file" -f "$compose_file" "$@"
}

dump_diagnostics() {
  echo "=== docker compose ps -a ===" >&2
  compose ps -a >&2 || true
  echo "=== docker compose logs (tail 200) ===" >&2
  compose logs --no-color --tail=200 >&2 || true
}

# `docker compose` recreates the project network whenever its definition changes,
# but it refuses to remove a network that still has containers attached and fails
# with "has active endpoints". Bring the stack down and retry once in that case;
# named volumes survive `compose down`.
compose_with_network_recovery() {
  local output status=0
  output="$(compose "$@" 2>&1)" || status=$?
  printf '%s\n' "$output"
  if [[ "$status" -ne 0 ]] && grep -q 'has active endpoints' <<<"$output"; then
    echo "project network must be replaced; bringing the stack down first" >&2
    compose down --remove-orphans
    compose "$@"
    return
  fi
  return "$status"
}

compose pull
compose_with_network_recovery run --rm volume-permissions
compose run --rm migrate

if ! compose up -d --remove-orphans; then
  echo "docker compose up failed" >&2
  dump_diagnostics
  exit 1
fi

compose ps
