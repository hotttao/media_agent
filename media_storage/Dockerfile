FROM golang:1.26-alpine AS builder
RUN apk add --no-cache git ca-certificates
ENV GOPROXY=https://goproxy.cn,direct
ENV GOSUMDB=off
WORKDIR /workspace
COPY hertz_gen/go.mod hertz_gen/go.sum hertz_gen/
COPY hertz_infra/go.mod hertz_infra/go.sum hertz_infra/
COPY media_storage/go.mod media_storage/go.sum media_storage/
RUN cd media_storage && GOWORK=off go mod download
COPY hertz_gen/ hertz_gen/
COPY hertz_infra/ hertz_infra/
COPY media_storage/ media_storage/
RUN cd media_storage && GOWORK=off CGO_ENABLED=0 go build -o /out/media_storage .

FROM alpine:3.20
RUN apk add --no-cache ca-certificates tzdata
WORKDIR /app
COPY --from=builder /out/media_storage /app/media_storage
COPY media_storage/migrations/ /app/migrations/
COPY media_storage/conf/ /app/conf/
COPY media_storage/.env /app/.env
EXPOSE 8008
ENTRYPOINT ["/app/media_storage"]
