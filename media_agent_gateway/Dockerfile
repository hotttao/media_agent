FROM golang:1.26-alpine AS builder
RUN apk add --no-cache git ca-certificates
ENV GOPROXY=https://goproxy.cn,direct
ENV GOSUMDB=off
WORKDIR /workspace
COPY hertz_gen/go.mod hertz_gen/go.sum hertz_gen/
COPY hertz_infra/go.mod hertz_infra/go.sum hertz_infra/
COPY media_agent_gateway/go.mod media_agent_gateway/go.sum media_agent_gateway/
RUN cd media_agent_gateway && GOWORK=off go mod download
COPY hertz_gen/ hertz_gen/
COPY hertz_infra/ hertz_infra/
COPY media_agent_gateway/ media_agent_gateway/
RUN cd media_agent_gateway && GOWORK=off CGO_ENABLED=0 go build -o /out/media_agent_gateway .

FROM alpine:3.20
RUN apk add --no-cache ca-certificates tzdata
WORKDIR /app
COPY --from=builder /out/media_agent_gateway /app/media_agent_gateway
COPY media_agent_gateway/migrations/ /app/migrations/
COPY media_agent_gateway/conf/ /app/conf/
COPY media_agent_gateway/.env /app/.env
EXPOSE 8006
ENTRYPOINT ["/app/media_agent_gateway"]
