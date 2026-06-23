# Workspace 根 Makefile
# 收口跨服务共享的代码生成动作（model + client）。
#
# 共享生成根：hertz_gen/（独立 module media_agent/hertz_gen），
# 所有服务共用：
#   - hertz_gen/model/        → API DTO（hz model 生成）
#   - hertz_gen/<service>/    → 下游 client stub（hz client 生成）
#
# IDL 源：idl/<service>/<file>.proto
# 用法：
#   make hz-model
#   make hz-client
# 自动遍历 idl/ 下所有业务 proto（排除 google/ openapiv3/ config/ 基础依赖），
# 基础 proto（api.proto、google/*、openapiv3/* 等）作为依赖由 hz 工具自动生成。

HERTZ_GEN_MODULE ?= media_agent/hertz_gen
HERTZ_GEN_MODEL_PKG ?= model
# --use 路径（caller 端 hz update / hz client 引用共享 model）
HERTZ_GEN_MODEL ?= $(HERTZ_GEN_MODULE)/$(HERTZ_GEN_MODEL_PKG)

# 业务 IDL：idl 下所有叶子 proto，过滤掉 google/ openapiv3/ config 等基础依赖目录
BIZ_PROTOS := $(shell find idl -type f -name '*.proto' \
		-not -path 'idl/google/*' \
		-not -path 'idl/openapiv3/*' \
		-not -path 'idl/config/*' \
		-not -name 'api.proto' \
		| sed 's|^idl/||')

.PHONY: hz-model
# hz-model：遍历所有业务 IDL，仅生成 model，写入共享 hertz_gen module。
hz-model:
	@cd hertz_gen && for p in $(BIZ_PROTOS); do \
		echo ">> hz model $$p"; \
		hz model \
			--module $(HERTZ_GEN_MODULE) \
			--model_dir $(HERTZ_GEN_MODEL_PKG) \
			--idl ../idl/$$p \
			--proto_path=../idl || exit 1; \
	done

.PHONY: hz-client
# hz-client：遍历所有业务 IDL，为下游服务生成 client stub。
hz-client:
	@cd hertz_gen && for p in $(BIZ_PROTOS); do \
		SERVICE=$$(dirname $$p | tr '/' '_'); \
		echo ">> hz client $$p -> $$SERVICE"; \
		hz client \
			--module $(HERTZ_GEN_MODULE) \
			--idl ../idl/$$p \
			--proto_path=../idl \
			--client_dir=$$SERVICE \
			--use $(HERTZ_GEN_MODEL) || exit 1; \
	done

.PHONY: pb-config
# pb-config：编译 idl/config/config.proto 到 hertz_gen/config/。
# 配置 schema 跨服务共享，走纯 protoc + protoc-gen-go，不走 hz 工具。
pb-config:
	protoc --go_out=. --go_opt=module=media_agent \
		--proto_path=idl \
		idl/config/config.proto
