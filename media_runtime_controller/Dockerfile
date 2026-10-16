FROM golang:1.26-alpine AS build
ENV GOMAXPROCS=2 GOPROXY=https://goproxy.cn,direct
WORKDIR /src
COPY hertz_infra ./hertz_infra
COPY hertz_gen ./hertz_gen
COPY media_runtime_controller ./media_runtime_controller
WORKDIR /src/media_runtime_controller

FROM build AS docker-build
RUN --mount=type=cache,target=/root/.cache/go-build CGO_ENABLED=0 GOWORK=off go build -o /out/runtime-controller-docker ./cmd/docker
FROM alpine:3.20 AS docker
USER 65532:65532
COPY --from=docker-build /out/runtime-controller-docker /runtime-controller-docker
ENTRYPOINT ["/runtime-controller-docker"]

FROM build AS kubernetes-build
RUN --mount=type=cache,target=/root/.cache/go-build CGO_ENABLED=0 GOWORK=off go build -o /out/runtime-controller-kubernetes ./cmd/kubernetes
FROM alpine:3.20 AS kubernetes
USER 65532:65532
COPY --from=kubernetes-build /out/runtime-controller-kubernetes /runtime-controller-kubernetes
ENTRYPOINT ["/runtime-controller-kubernetes"]
