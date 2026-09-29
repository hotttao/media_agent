#!/usr/bin/env bash
# Keep Vite alive across terminal sessions and user-manager restarts.
set -euo pipefail

project_dir="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.." && pwd)"
template=media-agent-ui-workflow-vite@.service
unit="$(systemd-escape --path --template="$template" "$project_dir")"
action="${1:-start}"

case "$action" in
  start)
    # Migrate the old transient unit before claiming the same listening port.
    if systemctl --user is-active --quiet media-agent-ui-workflow-vite.service; then
      systemctl --user stop media-agent-ui-workflow-vite.service
    fi
    systemctl --user link "$project_dir/scripts/$template"
    systemctl --user daemon-reload
    systemctl --user enable --now "$unit"
    printf 'Persistent Vite service enabled.\n'
    ;;
  status)
    exec systemctl --user status "$unit" --no-pager
    ;;
  logs)
    exec journalctl --user --unit="$unit" --no-pager -n 80
    ;;
  stop)
    # Explicit stop also opts out of automatic startup at the next login/boot.
    exec systemctl --user disable --now "$unit"
    ;;
  *)
    printf 'Usage: bash scripts/dev-service.sh [start|status|logs|stop]\n' >&2
    exit 2
    ;;
esac
