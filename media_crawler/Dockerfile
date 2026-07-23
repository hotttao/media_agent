FROM golang:1.26-alpine AS builder
RUN apk add --no-cache git ca-certificates
ENV GOPROXY=https://goproxy.cn,direct
ENV GOSUMDB=off
WORKDIR /workspace
COPY hertz_gen/go.mod hertz_gen/go.sum hertz_gen/
COPY hertz_infra/go.mod hertz_infra/go.sum hertz_infra/
COPY media_crawler/go.mod media_crawler/go.sum media_crawler/
RUN cd media_crawler && GOWORK=off go mod download
COPY hertz_gen/ hertz_gen/
COPY hertz_infra/ hertz_infra/
COPY media_crawler/ media_crawler/
RUN cd media_crawler && GOWORK=off CGO_ENABLED=0 go build -o /out/media_crawler .

FROM alpine:3.20
RUN apk add --no-cache ca-certificates tzdata
WORKDIR /app
COPY --from=builder /out/media_crawler /app/media_crawler
COPY media_crawler/migrations/ /app/migrations/
COPY media_crawler/conf/ /app/conf/
COPY media_crawler/.env /app/.env
EXPOSE 8004
ENTRYPOINT ["/app/media_crawler"]
