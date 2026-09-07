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

docker compose --env-file "$env_file" -f "$compose_file" pull
docker compose --env-file "$env_file" -f "$compose_file" run --rm migrate
docker compose --env-file "$env_file" -f "$compose_file" up -d --remove-orphans
docker compose --env-file "$env_file" -f "$compose_file" ps
