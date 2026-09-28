#!/usr/bin/env bash
# Keep the local Vite process alive independently of a terminal/tool session.
set -euo pipefail

project_dir="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.." && pwd)"
unit=media-agent-ui-workflow-vite.service
action="${1:-start}"

case "$action" in
  start)
    if systemctl --user is-active --quiet "$unit"; then
      printf 'Vite is already running.\n'
    elif [ "$(systemctl --user show "$unit" --property=LoadState --value)" != not-found ]; then
      systemctl --user start "$unit"
    else
      systemd-run --user --unit="$unit" \
        --property="WorkingDirectory=$project_dir" \
        --property=Restart=on-failure --property=RestartSec=3 \
        --setenv="PATH=$PATH" \
        /usr/bin/env npm run dev -- --host 0.0.0.0 --port 5173 --strictPort
    fi
    ;;
  status)
    exec systemctl --user status "$unit" --no-pager
    ;;
  logs)
    exec journalctl --user --unit="$unit" --no-pager -n 80
    ;;
  stop)
    exec systemctl --user stop "$unit"
    ;;
  *)
    printf 'Usage: bash scripts/dev-service.sh [start|status|logs|stop]\n' >&2
    exit 2
    ;;
esac
