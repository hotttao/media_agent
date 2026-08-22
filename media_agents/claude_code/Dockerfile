FROM node:22-bookworm-slim AS node-tools

ARG CLAUDE_CODE_VERSION=2.1.231

COPY .npmrc /root/.npmrc
RUN npm install --global "@anthropic-ai/claude-code@${CLAUDE_CODE_VERSION}"

FROM python:3.12-slim

ARG UV_VERSION=0.8.12

COPY pip.conf /etc/pip.conf
RUN pip install --no-cache-dir "uv==${UV_VERSION}"
COPY --from=node-tools /usr/local/bin/node /usr/local/bin/node
COPY --from=node-tools /usr/local/lib/node_modules /usr/local/lib/node_modules
RUN ln -s ../lib/node_modules/@anthropic-ai/claude-code/bin/claude.exe /usr/local/bin/claude

WORKDIR /app/media_agents
COPY . ./
RUN uv sync --frozen --package claude-code --no-dev

RUN mkdir -p /workspace/.home/.claude

ENV PATH="/app/media_agents/.venv/bin:${PATH}" \
    PYTHONPATH="/app/media_agents" \
    UV_DEFAULT_INDEX="https://mirrors.aliyun.com/pypi/simple/" \
    HOME="/workspace/.home" \
    WORKSPACE_DIR="/workspace" \
    SESSION_STORE_DIR="/workspace/.home/.claude/projects" \
    PORT="3003"

EXPOSE 3003
CMD ["python", "-m", "claude_code.main"]
