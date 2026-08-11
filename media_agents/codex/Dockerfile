FROM node:22-bookworm-slim AS node-tools

ARG CODEX_ACP_VERSION=1.1.14
ARG CODEX_VERSION=0.147.0

COPY .npmrc /root/.npmrc
RUN npm install --global \
    "@agentclientprotocol/codex-acp@${CODEX_ACP_VERSION}" \
    "@openai/codex@${CODEX_VERSION}"

FROM python:3.12-slim-bookworm

ARG UV_VERSION=0.8.12

COPY pip.conf /etc/pip.conf
RUN pip install --no-cache-dir "uv==${UV_VERSION}"
COPY --from=node-tools /usr/local/bin/node /usr/local/bin/node
COPY --from=node-tools /usr/local/lib/node_modules /usr/local/lib/node_modules
COPY --from=node-tools /usr/local/bin/codex /usr/local/bin/codex
COPY --from=node-tools /usr/local/bin/codex-acp /usr/local/bin/codex-acp

WORKDIR /app/media_agents
COPY . ./
RUN uv sync --frozen --package codex --no-dev

RUN mkdir -p /root/.codex /workspace \
    && install -m 0600 codex/.docker-auth/auth.json /root/.codex/auth.json \
    && rm -rf codex/.docker-auth

ENV PATH="/app/media_agents/.venv/bin:${PATH}" \
    PYTHONPATH="/app/media_agents" \
    UV_DEFAULT_INDEX="https://mirrors.aliyun.com/pypi/simple/" \
    CODEX_CONFIG_DIR="/root/.codex" \
    WORKSPACE_DIR="/workspace" \
    MOCK_MEDIA_CONTROL="true" \
    CODEX_AUX_PROCESSES="false" \
    PORT="3001"

EXPOSE 3001
CMD ["python", "-m", "codex.main"]
