FROM python:3.12-slim

ARG UV_VERSION=0.8.12
ARG GOOSE_VERSION=1.52.0

RUN apt-get update \
    && apt-get install -y --no-install-recommends ca-certificates curl bzip2 \
    && rm -rf /var/lib/apt/lists/*
RUN curl --retry 5 --retry-all-errors --retry-delay 1 -fsSL \
    "https://github.com/aaif-goose/goose/releases/download/v${GOOSE_VERSION}/goose-$(uname -m)-unknown-linux-musl.tar.bz2" \
    -o /tmp/goose.tar.bz2 \
    && tar -xjf /tmp/goose.tar.bz2 -C /usr/local/bin \
    && chmod 755 /usr/local/bin/goose \
    && rm /tmp/goose.tar.bz2
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
