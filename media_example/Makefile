HERTZ_GEN_MODEL ?= media_agent/hertz_gen/model

.PHONY: run
run:
	go run .

.PHONY: test
test:
	go test ./...

# 架构检测：运行 workspace 级共享 archcheck。
.PHONY: arch-check
arch-check:
	go run ../harness/tools/archcheck

.PHONY: check
check: arch-check test

.PHONY: tidy
tidy:
	go mod tidy

.PHONY: fmt
fmt:
	gofmt -w .

# 业务 IDL：../idl/media_example/ 下所有 .proto（相对 IDL 根，去掉 ../idl/ 前缀）
BIZ_PROTOS := $(shell find ../idl/media_example -type f -name '*.proto' | sed 's|^\.\./idl/||')

.PHONY: hz-update
hz-update:
	@for p in $(BIZ_PROTOS); do \
		echo ">> hz update $$p"; \
		hz update \
			--idl ../idl/$$p \
			--proto_path=../idl \
			--use $(HERTZ_GEN_MODEL) \
			--sort_router || exit 1; \
	done

# hz-gen：一键重新生成 model + handler/router。
.PHONY: hz-gen
hz-gen: hz-update
