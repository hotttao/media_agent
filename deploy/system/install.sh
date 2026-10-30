#!/usr/bin/env bash
# Explicit Debian/Ubuntu prerequisite installation; not run during metadata work.
set -euo pipefail
command -v apt-get >/dev/null || { echo 'Only Debian/Ubuntu supported; see README' >&2; exit 1; }
sudo apt-get update
sudo apt-get install -y ca-certificates curl git python3 python3-venv python3-pip \
  build-essential pkg-config libnss3 libatk1.0-0 libatk-bridge2.0-0 libcups2 libdrm2 \
  libxkbcommon0 libxcomposite1 libxdamage1 libxfixes3 libxrandr2 libgbm1 xdg-utils
# libasound uses the t64 name on newer Ubuntu/Debian releases.
if apt-cache show libasound2t64 >/dev/null 2>&1; then
  sudo apt-get install -y libasound2t64
else
  sudo apt-get install -y libasound2
fi
echo 'OS prerequisites installed. Node, Chrome, Docker and Go are separate operator steps; see README.'
