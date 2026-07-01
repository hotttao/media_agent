#!/bin/bash
# auth_unauthorized_test.sh — 测试无 cookie 访问 /me → 401。
set -e

MEDIA_AUTH_HOST="${MEDIA_AUTH_ADDR:-http://localhost:8001}"

CODE=$(curl -s -o /dev/null -w "%{http_code}" "$MEDIA_AUTH_HOST/api/auth/me")

if [ "$CODE" != "401" ]; then
  echo "  expected 401, got $CODE"
  exit 1
fi
