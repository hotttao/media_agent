#!/bin/bash
# seed.sh — e2e 数据准备：seed 一个本地密码测试用户 alice/secret123。
#
# 通过 docker compose exec 在 pg 容器执行 SQL，用 argon2hash 工具生成 hash。
set -e

REPO_ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
COMPOSE_FILE="$REPO_ROOT/docker-compose/docker-compose.yml"

# 生成 argon2 hash（用 media_auth 的 argon2hash 工具）。
HASH=$(cd "$REPO_ROOT/media_auth" && go run ./tools/argon2hash secret123)
if [ -z "$HASH" ]; then
  echo "  failed to generate argon2 hash"
  exit 1
fi

# 在 pg 容器执行 SQL seed 用户（幂等：先删再插）。
docker compose -f "$COMPOSE_FILE" exec -T postgres \
  psql -U media_auth -d media_auth -c "
    DELETE FROM media_auth.users WHERE username = 'alice';
    INSERT INTO media_auth.users (id, username, display_name, password_hash, role)
    VALUES ('u1', 'alice', 'Alice', '$HASH', 'user')
    ON CONFLICT (id) DO UPDATE SET password_hash = EXCLUDED.password_hash;
  "

echo "seeded user alice/secret123"
