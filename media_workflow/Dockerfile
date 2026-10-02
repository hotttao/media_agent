FROM golang:1.26-alpine AS builder
RUN apk add --no-cache git ca-certificates
ENV GOPROXY=https://goproxy.cn,direct
ENV GOSUMDB=off
WORKDIR /workspace
COPY hertz_gen/go.mod hertz_gen/go.sum hertz_gen/
COPY hertz_infra/go.mod hertz_infra/go.sum hertz_infra/
COPY media_workflow/go.mod media_workflow/go.sum media_workflow/
RUN cd media_workflow && GOWORK=off go mod download
COPY hertz_gen/ hertz_gen/
COPY hertz_infra/ hertz_infra/
COPY media_workflow/ media_workflow/
RUN --mount=type=cache,target=/root/.cache/go-build cd media_workflow && GOWORK=off CGO_ENABLED=0 go build -o /out/media_workflow .
RUN --mount=type=cache,target=/root/.cache/go-build cd media_workflow && GOWORK=off CGO_ENABLED=0 go build -o /out/media_workflow_internal_worker ./cmd/internal_worker
RUN --mount=type=cache,target=/root/.cache/go-build cd media_workflow && GOWORK=off CGO_ENABLED=0 go build -o /out/media_workflow_managed_worker ./cmd/managed_worker

FROM alpine:3.20
RUN apk add --no-cache ca-certificates tzdata
WORKDIR /app
COPY --from=builder /out/media_workflow /app/media_workflow
COPY --from=builder /out/media_workflow_internal_worker /app/media_workflow_internal_worker
COPY --from=builder /out/media_workflow_managed_worker /app/media_workflow_managed_worker
COPY media_workflow/migrations/ /app/migrations/
COPY media_workflow/conf/ /app/conf/
COPY media_workflow/.env /app/.env
EXPOSE 8002
ENTRYPOINT ["/app/media_workflow"]
