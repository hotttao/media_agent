#!/usr/bin/env bash
source "$(dirname "${BASH_SOURCE[0]}")/../lib.sh"
need curl; need sha256sum
# Pin the installer as well as the release; never execute a floating main script.
version=v1.5.266
expected=af021fa8b0b89ea27ab43eab68427c2c69f40befd52c030200ecd100432a34fd
tmp=$(mktemp)
trap 'rm -f "$tmp"' EXIT
curl --fail --location --retry 3 "https://raw.githubusercontent.com/leeguooooo/chrome-use/$version/install.sh" -o "$tmp"
printf '%s  %s\n' "$expected" "$tmp" | sha256sum --check --status
AGENT_BROWSER_VERSION="$version" AGENT_BROWSER_BIN_DIR="$DEPLOY_ROOT/chrome-use/bin" \
  AGENT_BROWSER_NO_SETUP=1 AGENT_BROWSER_NO_SKILL=1 sh "$tmp"
link_command "$DEPLOY_ROOT/chrome-use/bin/chrome-use" chrome-use
printf 'Native host setup (run yourself): %s extension install\n' "$DEPLOY_ROOT/chrome-use/bin/chrome-use"
