#!/bin/bash
# auth_login_me_test.sh — 测试 auth login → me 流程（cookie 跨请求复用）。
set -e

MEDIA_AUTH_HOST="${MEDIA_AUTH_ADDR:-http://localhost:8001}"
JAR=$(mktemp)

# 1. login
RESP=$(curl -s -c "$JAR" -w "\n%{http_code}" -X POST "$MEDIA_AUTH_HOST/api/auth/login" \
  -H "Content-Type: application/json" \
  -d '{"username":"alice","password":"secret123"}')
BODY=$(echo "$RESP" | head -n -1)
CODE=$(echo "$RESP" | tail -1)

if [ "$CODE" != "200" ]; then
  echo "  login failed: status=$CODE body=$BODY"
  exit 1
fi
if ! echo "$BODY" | grep -q '"username":"alice"'; then
  echo "  login response missing username: $BODY"
  exit 1
fi

# 2. me（cookie 自动带）
RESP=$(curl -s -b "$JAR" -w "\n%{http_code}" "$MEDIA_AUTH_HOST/api/auth/me")
BODY=$(echo "$RESP" | head -n -1)
CODE=$(echo "$RESP" | tail -1)

if [ "$CODE" != "200" ]; then
  echo "  me failed: status=$CODE body=$BODY"
  exit 1
fi
if ! echo "$BODY" | grep -q '"username":"alice"'; then
  echo "  me response missing username: $BODY"
  exit 1
fi

rm -f "$JAR"
