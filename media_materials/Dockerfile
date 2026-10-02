FROM golang:1.26-alpine AS builder
RUN apk add --no-cache git ca-certificates
ENV GOPROXY=https://goproxy.cn,direct
ENV GOSUMDB=off
WORKDIR /workspace
COPY hertz_gen/go.mod hertz_gen/go.sum hertz_gen/
COPY hertz_infra/go.mod hertz_infra/go.sum hertz_infra/
COPY media_materials/go.mod media_materials/go.sum media_materials/
RUN cd media_materials && GOWORK=off go mod download
COPY hertz_gen/ hertz_gen/
COPY hertz_infra/ hertz_infra/
COPY media_materials/ media_materials/
RUN --mount=type=cache,target=/root/.cache/go-build cd media_materials && GOWORK=off CGO_ENABLED=0 go build -o /out/media_materials .

FROM alpine:3.20
RUN apk add --no-cache ca-certificates tzdata
WORKDIR /app
COPY --from=builder /out/media_materials /app/media_materials
COPY media_materials/migrations/ /app/migrations/
COPY media_materials/conf/ /app/conf/
COPY media_materials/.env /app/.env
EXPOSE 8003
ENTRYPOINT ["/app/media_materials"]
