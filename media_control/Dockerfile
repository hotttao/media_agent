FROM golang:1.26-alpine AS builder
RUN apk add --no-cache git ca-certificates
ENV GOPROXY=https://goproxy.cn,direct
ENV GOSUMDB=off
ENV GOMAXPROCS=2
WORKDIR /workspace
COPY hertz_gen/go.mod hertz_gen/go.sum hertz_gen/
COPY hertz_infra/go.mod hertz_infra/go.sum hertz_infra/
COPY media_control/go.mod media_control/go.sum media_control/
RUN cd media_control && GOWORK=off go mod download
COPY hertz_gen/ hertz_gen/
COPY hertz_infra/ hertz_infra/
COPY media_control/ media_control/
RUN --mount=type=cache,target=/root/.cache/go-build cd media_control && GOWORK=off CGO_ENABLED=0 go build -o /out/media_control .

FROM alpine:3.20
RUN apk add --no-cache ca-certificates tzdata git
WORKDIR /app
COPY --from=builder /out/media_control /app/media_control
COPY media_control/migrations/ /app/migrations/
COPY media_control/conf/ /app/conf/
ENV APP_ENV=local
EXPOSE 8007
ENTRYPOINT ["/app/media_control"]
