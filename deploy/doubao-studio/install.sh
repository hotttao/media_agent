#!/usr/bin/env bash
source "$(dirname "${BASH_SOURCE[0]}")/../lib.sh"
need node; need npm
revision=${STUDIO_REVISION:-15e2e7e8e7820626ed1f8433f80ed01d9e047ce1}
repository=${STUDIO_REPOSITORY:-https://github.com/hotttao/doubao-studio.git}
destination="$DEPLOY_ROOT/doubao-studio/source"
need git
if [[ ! -e "$destination" ]]; then
  mkdir -p "$(dirname "$destination")"
  git clone --no-checkout "$repository" "$destination"
fi
[[ -d "$destination/.git" ]] || { echo 'Destination must be a Git checkout' >&2; exit 1; }
[[ -z $(git -C "$destination" status --porcelain) ]] || { echo "Dirty checkout: $destination" >&2; exit 1; }
[[ $(git -C "$destination" remote get-url origin) == "$repository" ]] || { echo 'Origin mismatch' >&2; exit 1; }
git -C "$destination" fetch origin "$revision"
git -C "$destination" checkout --detach FETCH_HEAD
# A private pnpm prefix avoids modifying global npm/Corepack configuration.
npm install --global --prefix "$DEPLOY_ROOT/doubao-studio/toolchain" pnpm@9.15.9
export PATH="$DEPLOY_ROOT/doubao-studio/toolchain/bin:$PATH"
cd "$DEPLOY_ROOT/doubao-studio/source"
pnpm install --frozen-lockfile
pnpm run build
printf 'Studio built at %s; no process started.\n' "$PWD"
