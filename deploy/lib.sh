#!/usr/bin/env bash
# Shared paths only. Sourcing this file has no filesystem or service side effects.
set -euo pipefail
DEPLOY_METADATA=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
MEDIA_AGENT_SOURCE=$(cd "$DEPLOY_METADATA/.." && pwd)
DEPLOY_ROOT=${DEPLOY_ROOT:-/home/tao/deploy}
DATA_ROOT=${DATA_ROOT:-/data}
case "$DEPLOY_ROOT:$DATA_ROOT" in /*:/*) ;; *) echo 'DEPLOY_ROOT and DATA_ROOT must be absolute' >&2; exit 1;; esac
need() { command -v "$1" >/dev/null || { echo "Required command: $1" >&2; exit 1; }; }
# Explicit installs never overwrite another command in ~/.local/bin.
link_command() {
  local target=$1 name=$2
  mkdir -p "$HOME/.local/bin"
  if [[ -e "$HOME/.local/bin/$name" || -L "$HOME/.local/bin/$name" ]]; then
    if [[ $(readlink "$HOME/.local/bin/$name" || true) != "$target" ]]; then
      echo "Keeping existing $HOME/.local/bin/$name; new command: $target"
      return
    fi
  fi
  ln -sfn "$target" "$HOME/.local/bin/$name"
}
