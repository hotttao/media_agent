FROM golang:1.26-alpine AS builder
RUN apk add --no-cache git ca-certificates
ENV GOPROXY=https://goproxy.cn,direct
ENV GOSUMDB=off
WORKDIR /workspace
COPY hertz_gen/go.mod hertz_gen/go.sum hertz_gen/
COPY hertz_infra/go.mod hertz_infra/go.sum hertz_infra/
COPY media_gen/go.mod media_gen/go.sum media_gen/
RUN cd media_gen && GOWORK=off go mod download
COPY hertz_gen/ hertz_gen/
COPY hertz_infra/ hertz_infra/
COPY media_gen/ media_gen/
RUN cd media_gen && GOWORK=off CGO_ENABLED=0 go build -o /out/media_gen .

FROM alpine:3.20
RUN apk add --no-cache ca-certificates tzdata
WORKDIR /app
COPY --from=builder /out/media_gen /app/media_gen
COPY media_gen/migrations/ /app/migrations/
COPY media_gen/conf/ /app/conf/
COPY media_gen/.env /app/.env
EXPOSE media_gen
ENTRYPOINT ["/app/8005"]
%!(EXTRA string=media_gen)