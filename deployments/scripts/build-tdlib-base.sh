#!/usr/bin/env bash
set -euo pipefail

: "${CLOUDRU_REGISTRY:?CLOUDRU_REGISTRY is required}"

image="${CLOUDRU_REGISTRY}/content_scout-tdlib-base:${TDLIB_BASE_TAG:-latest}"

docker build \
  -f deployments/docker/tdlib-base.Dockerfile \
  -t "$image" \
  .

docker push "$image"
