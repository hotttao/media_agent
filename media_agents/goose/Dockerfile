FROM python:3.12-slim

ARG UV_VERSION=0.8.12
ARG GOOSE_VERSION=1.52.0

RUN apt-get update \
    && apt-get install -y --no-install-recommends ca-certificates curl bzip2 \
    && rm -rf /var/lib/apt/lists/*
RUN curl -fsSL "https://raw.githubusercontent.com/aaif-goose/goose/v${GOOSE_VERSION}/download_cli.sh" -o /tmp/install-goose.sh \
    && GOOSE_VERSION="${GOOSE_VERSION}" GOOSE_BIN_DIR=/usr/local/bin GOOSE_LINUX_VARIANT=musl CONFIGURE=false bash /tmp/install-goose.sh \
    && rm /tmp/install-goose.sh
COPY pip.conf /etc/pip.conf
RUN pip install --no-cache-dir "uv==${UV_VERSION}"
WORKDIR /app/media_agents
COPY . ./
RUN uv sync --frozen --package goose --no-dev
ENV PATH="/app/media_agents/.venv/bin:${PATH}" \
    PYTHONPATH="/app/media_agents" \
    HOME="/workspace/.home" \
    GOOSE_CONFIG_DIR="/workspace/.home/.config/goose" \
    WORKSPACE_DIR="/workspace" \
    PORT="3002"
EXPOSE 3002
CMD ["python", "-m", "goose.main"]
