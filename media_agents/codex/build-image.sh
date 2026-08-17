#!/usr/bin/env bash
set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
image_tag="${CODEX_IMAGE_TAG:-media-agent-codex:local}"

docker build --file "${repo_root}/media_agents/codex/Dockerfile" --tag "${image_tag}" "${repo_root}/media_agents"
