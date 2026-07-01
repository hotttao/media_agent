# media_example Dockerfile —— 多阶段 build（workspace 多 module）。
#
# 构建上下文必须是仓库根（go.work 需要 hertz_gen/hertz_infra 等 replace module）。
# 用法：docker compose build media_example（compose 的 build context 已设为仓库根）。

# ── builder ──
FROM golang:1.26-alpine AS builder

RUN apk add --no-cache git ca-certificates

# 国内 Go 代理（goproxy.cn）。
ENV GOPROXY=https://goproxy.cn,direct
ENV GOSUMDB=off

WORKDIR /workspace

# 先 copy go.mod/go.sum（利用 layer cache：代码变了不重新下载依赖）。
COPY hertz_gen/go.mod hertz_gen/go.sum hertz_gen/
COPY hertz_infra/go.mod hertz_infra/go.sum hertz_infra/
COPY media_example/go.mod media_example/go.sum media_example/

# go mod download（GOWORK=off：避免 go.work 加载不存在的模块）。
RUN cd media_example && GOWORK=off go mod download

# 再 copy 源码（代码变更只触发重新 build，不触发 go mod download）。
COPY hertz_gen/ hertz_gen/
COPY hertz_infra/ hertz_infra/
COPY media_example/ media_example/

# build 二进制。
RUN cd media_example && GOWORK=off CGO_ENABLED=0 go build -o /out/media_example .

# ── runtime ──
FROM alpine:3.20

RUN apk add --no-cache ca-certificates tzdata

WORKDIR /app
COPY --from=builder /out/media_example /app/media_example
# migrations 目录（运行时读磁盘迁移，不 embed）。
COPY media_example/migrations/ /app/migrations/
# conf 目录（容器内用 local 配置，环境变量覆盖）。
COPY media_example/conf/ /app/conf/
COPY media_example/.env /app/.env

EXPOSE 8000

ENTRYPOINT ["/app/media_example"]
