FROM golang:1.26-alpine AS builder
RUN apk add --no-cache git ca-certificates
ENV GOPROXY=https://goproxy.cn,direct
WORKDIR /workspace
COPY hertz_gen/go.mod hertz_gen/go.sum hertz_gen/
COPY hertz_infra/go.mod hertz_infra/go.sum hertz_infra/
COPY media_plugin/go.mod media_plugin/go.sum media_plugin/
RUN cd media_plugin && GOWORK=off go mod download
COPY hertz_gen/ hertz_gen/
COPY hertz_infra/ hertz_infra/
COPY media_plugin/ media_plugin/
RUN --mount=type=cache,target=/root/.cache/go-build cd media_plugin && GOWORK=off CGO_ENABLED=0 go build -o /out/media_plugin .
FROM alpine:3.20
RUN apk add --no-cache ca-certificates tzdata
WORKDIR /app
COPY --from=builder /out/media_plugin /app/media_plugin
COPY media_plugin/migrations/ /app/migrations/
COPY media_plugin/conf/ /app/conf/
COPY media_plugin/.env /app/.env
EXPOSE 8011
ENTRYPOINT ["/app/media_plugin"]
