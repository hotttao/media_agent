#!/usr/bin/env bash
# prepare.sh — 启动观测栈前创建 /data/<service> 目录并设置权限。
#
# 每个 infra 服务的数据和日志各落自己目录（/data/<service>/）；
# app 日志落 ../media_logs/<app>/（已存在，此处确保可写）。
#
# 用法：cd docker-compose && ./prepare.sh

set -euo pipefail

DATA_ROOT="${DATA_ROOT:-/data}"
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
MEDIA_LOGS="$SCRIPT_DIR/../media_logs"

services=(
  otel-collector
  jaeger
  prometheus
  loki
  grafana
)

echo ">> creating infra data dirs under ${DATA_ROOT}"
for svc in "${services[@]}"; do
  dir="${DATA_ROOT}/${svc}"
  mkdir -p "${dir}"
  # 容器内进程常以非 root（如 grafana uid 472）运行，放宽目录权限避免写失败。
  chmod 0777 "${dir}"
  echo "   ${dir}"
done

# jaeger badger 子目录
mkdir -p "${DATA_ROOT}/jaeger/badger"
chmod 0777 "${DATA_ROOT}/jaeger/badger"

echo ">> ensuring app log dir writable: ${MEDIA_LOGS}"
mkdir -p "${MEDIA_LOGS}/media_example"
chmod 0777 "${MEDIA_LOGS}" "${MEDIA_LOGS}/media_example"

echo ">> done. run: docker compose -f ${SCRIPT_DIR}/docker-compose.yml up -d"
