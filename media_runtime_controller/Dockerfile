FROM golang:1.26.3 AS build
WORKDIR /src
COPY hertz_infra ./hertz_infra
COPY hertz_gen ./hertz_gen
COPY media_runtime_controller ./media_runtime_controller
WORKDIR /src/media_runtime_controller
RUN CGO_ENABLED=0 GOWORK=off go build -o /out/runtime-controller-kubernetes ./cmd/kubernetes
RUN CGO_ENABLED=0 GOWORK=off go build -o /out/runtime-controller-docker ./cmd/docker
FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/runtime-controller-kubernetes /runtime-controller-kubernetes
COPY --from=build /out/runtime-controller-docker /runtime-controller-docker
ENTRYPOINT ["/runtime-controller-kubernetes"]
