#!/bin/sh
# entrypoint.sh — 合并镜像启动入口
# 1) 把 pro-python 的运行时配置先 export 一遍 (uv.lock 里 .env 不会读, 但 python 端 cmd_arg.parse_cmd 会读 sys.argv)
# 2) 启动 python-proxy (Go), 它会按需 spawn python subprocess
set -e

# 兜底: CRAWLER_WORK_DIR / PYTHON_PATH / MCP_RUNNER_PATH 默认值 (manifest 里已经设了, 这里再保险)
export CRAWLER_WORK_DIR="${CRAWLER_WORK_DIR:-/app}"
export PYTHON_PATH="${PYTHON_PATH:-/app/.venv/bin/python}"
export MCP_RUNNER_PATH="${MCP_RUNNER_PATH:-/app/script/mcp_runner.py}"

# python-proxy 需要 /app 在 sys.path, 检查关键文件
if [ ! -x "$PYTHON_PATH" ]; then
  echo "[entrypoint] FATAL: $PYTHON_PATH not executable" >&2
  exit 1
fi
if [ ! -f "$MCP_RUNNER_PATH" ]; then
  echo "[entrypoint] FATAL: $MCP_RUNNER_PATH not found" >&2
  exit 1
fi
if [ ! -f "$CRAWLER_WORK_DIR/main.py" ]; then
  echo "[entrypoint] FATAL: $CRAWLER_WORK_DIR/main.py not found (pro-python project root missing)" >&2
  exit 1
fi

echo "[entrypoint] CRAWLER_WORK_DIR=$CRAWLER_WORK_DIR"
echo "[entrypoint] PYTHON_PATH=$PYTHON_PATH"
echo "[entrypoint] MCP_RUNNER_PATH=$MCP_RUNNER_PATH"
echo "[entrypoint] SIGN_SRV_HOST=${SIGN_SRV_HOST}:${SIGN_SRV_PORT}"
echo "[entrypoint] COOKIE_BRIDGE_URL=$COOKIE_BRIDGE_URL"
echo "[entrypoint] SERVER_ADDR=$SERVER_ADDR"

# 透传 CMD
exec "$@"