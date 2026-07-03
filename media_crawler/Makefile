HERTZ_GEN_MODEL ?= media_agent/hertz_gen/model

.PHONY: run
run:
	go run .

.PHONY: test
test:
	go test ./...

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

# mcp_*.proto 只含 message（无 service/rpc），仅走 hz-model 生成 struct；
# 不走 hz-update（否则生成空 router，且 mcp_xhs.Register 含子串 xhs.Register 误判重复注册）。
BIZ_PROTOS := $(shell find ../idl/media_crawler -type f -name '*.proto' ! -name 'mcp_*.proto' | sed 's|^\.\./idl/||')

.PHONY: hz-update
hz-update:
	@for p in $(BIZ_PROTOS); do \
		echo ">> hz update $$p"; \
		hz update --idl ../idl/$$p --proto_path ../idl --use $(HERTZ_GEN_MODEL) --sort_router || exit 1; \
	done

.PHONY: hz-gen
hz-gen: hz-update

.PHONY: swagger-gen
swagger-gen:
	@mkdir -p swagger
	protoc --proto_path=../idl --openapi_out=swagger --openapi_opt=naming=proto $(BIZ_PROTOS)

.PHONY: gorm-gen
gorm-gen:
	@test -n "$$MEDIA_CRAWLER_DB_DSN" || { echo "usage: MEDIA_CRAWLER_DB_DSN=postgres://... CRAWL_DATA_DSN=mysql://... make gorm-gen"; exit 1; }
	@test -n "$$CRAWL_DATA_DSN" || { echo "CRAWL_DATA_DSN (爬虫 MySQL 只读) required"; exit 1; }
	go run tools/gormgen/main.go

.PHONY: migrate-new
migrate-new:
	@test -n "$(NAME)" || { echo "usage: make migrate-new NAME=add_xxx"; exit 1; }
	migrate create -ext sql -dir migrations -seq $(NAME)

.PHONY: migrate-up
migrate-up:
	migrate -path migrations -database "$$PG_DSN" up

.PHONY: migrate-down
migrate-down:
	migrate -path migrations -database "$$PG_DSN" down 1

.PHONY: lefthook-install
lefthook-install:
	go install github.com/evilmartians/lefthook@latest
	lefthook install
