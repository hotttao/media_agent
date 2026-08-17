#!/bin/sh
set -eu

auth_source="${CODEX_AUTH_SOURCE:-/run/secrets/codex-auth.json}"
config_dir="${CODEX_CONFIG_DIR:-/root/.codex}"

mkdir -p "$config_dir" "${WORKSPACE_DIR:-/workspace}"
if [ -f "$auth_source" ]; then
  install -m 0600 "$auth_source" "$config_dir/auth.json"
fi

exec "$@"
