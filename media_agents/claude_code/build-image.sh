#!/bin/sh
set -eu

repo_root=$(CDPATH= cd -- "$(dirname -- "$0")/../.." && pwd)
image_tag="${CLAUDE_CODE_IMAGE_TAG:-media-agent-claude-code:local}"

docker build --file "${repo_root}/media_agents/claude_code/Dockerfile" --tag "${image_tag}" "${repo_root}/media_agents"
