#!/usr/bin/env bash
set -euo pipefail

: "${CLOUDRU_REGISTRY:?CLOUDRU_REGISTRY is required}"

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
repo_root="$(cd "$script_dir/../.." && pwd)"
image="${CLOUDRU_REGISTRY}/content_scout-tdlib-base:${TDLIB_BASE_TAG:-latest}"

docker build \
  -f "$repo_root/deployments/docker/tdlib-base.Dockerfile" \
  -t "$image" \
  "$repo_root"

docker push "$image"
