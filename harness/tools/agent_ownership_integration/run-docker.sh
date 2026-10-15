#!/usr/bin/env bash
# Isolated, resource-limited Docker-native protocol validation; no k3s dependency.
set -euo pipefail
repo_root=$(cd "$(dirname "${BASH_SOURCE[0]}")/../../.." && pwd)
cd "$repo_root"
project="media-agent-p8-$(id -u)-$$"
export P8_DOCKER_NETWORK="$project"
compose=(docker compose -p "$project" -f harness/tools/agent_ownership_integration/compose.docker.yaml)
cleanup() {
    result=$?
    trap - EXIT
    "${compose[@]}" down --volumes --timeout 5
    exit "$result"
}
trap cleanup EXIT
trap 'exit 130' INT
trap 'exit 143' TERM

docker build --memory=512m \
    --build-arg "BASE_AGENT_IMAGE=${MEDIA_RUNTIME_DOCKER_PIPELINE_BASE_IMAGE:-media-agent-codex:local}" \
    -f media_agents/scripts/Dockerfile.pipeline -t media-agent-pipeline:p8 media_agents
"${compose[@]}" up -d --wait --wait-timeout 240
export MEDIA_RUNTIME_DOCKER_PIPELINE_TEST=1
export MEDIA_RUNTIME_DOCKER_PIPELINE_NETWORK="$P8_DOCKER_NETWORK"
export MEDIA_RUNTIME_DOCKER_PIPELINE_IMAGE=media-agent-pipeline:p8
export MEDIA_GATEWAY_OWNERSHIP_TEST_DSN="postgres://postgres:p8fixture@127.0.0.1:${P8_POSTGRES_PORT:-25432}/agent_runtime?sslmode=disable"
export MEDIA_AGENTS_TEST_RABBITMQ_URL="amqp://guest:guest@127.0.0.1:${P8_RABBIT_PORT:-25672}/"
export GOMAXPROCS=2
go test -p 2 -count=1 -timeout 240s -v \
    -run '^TestDockerNativeCreationToSessionExecution$' ./harness/tools/agent_ownership_integration
