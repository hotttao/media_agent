HERTZ_GEN_MODEL ?= media_agent/hertz_gen/model
BIZ_PROTOS := $(wildcard ../idl/media_runtime_controller/*.proto)
.PHONY: test build fmt hz-update swagger-gen
test:
	go test ./...
build:
	go build ./cmd/kubernetes ./cmd/docker
fmt:
	gofmt -w .
hz-update:
	@for p in $(BIZ_PROTOS); do hz update --idl $$p --proto_path ../idl --use $(HERTZ_GEN_MODEL) --sort_router || exit 1; done
swagger-gen:
	@mkdir -p swagger
	protoc --proto_path=../idl --openapi_out=swagger --openapi_opt=naming=proto $(BIZ_PROTOS)
