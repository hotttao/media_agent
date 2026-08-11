#!/usr/bin/env bash
set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
auth_source="${CODEX_AUTH_FILE:-/home/tao/.codex/auth.json}"
auth_context="${repo_root}/media_agents/codex/.docker-auth"
image_tag="${CODEX_IMAGE_TAG:-media-agent-codex:local}"

if [[ ! -f "${auth_source}" ]]; then
  echo "Codex auth file not found: ${auth_source}" >&2
  exit 1
fi

mkdir -p "${auth_context}"
trap 'rm -rf "${auth_context}"' EXIT
install -m 0600 "${auth_source}" "${auth_context}/auth.json"
docker build --file "${repo_root}/media_agents/codex/Dockerfile" --tag "${image_tag}" "${repo_root}/media_agents"
