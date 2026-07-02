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

# 参与 OpenAPI 文档生成的正式 service proto：排除 test.proto（测试专用 service，不在正式
# 服务注册 handler，避免污染正式 API 文档）。约定：正式 service proto 必须同时挂
# api.* 与 google.api.http 两条注解（见 harness/contributing/idl.md §2）。
SWAGGER_PROTOS := $(shell find ../idl/media_example -type f -name '*.proto' \
		-not -name 'test.proto' | sed 's|^\.\./idl/||')

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

.PHONY: swagger-gen
# swagger-gen：用 protoc-gen-openapi（gnostic）从本服务业务 IDL 生成 OpenAPI v3 文档。
# 依赖：protoc-gen-openapi（go install github.com/google/gnostic/cmd/protoc-gen-openapi@latest）。
# 仅处理正式 service proto（SWAGGER_PROTOS，排除 test.proto）。每个 RPC 必须挂
# google.api.http 注解（idl.md §2），否则该 RPC 不会出现在 paths。
# 产物：swagger/openapi.yaml（router.go 经 go:embed 内嵌，/openapi.yaml 端点返回 +
# swagger-ui 加载；skill-gen 也以此为输入）。
swagger-gen:
	@mkdir -p swagger
	protoc --proto_path=../idl \
		--openapi_out=swagger \
		--openapi_opt=naming=proto \
		$(SWAGGER_PROTOS)

.PHONY: skill-gen
# skill-gen：基于 swagger openapi.yaml 生成 Claude skill 文档。
# 依赖：openapi-to-skills（系统已安装）；以及 swagger-gen 产出的 openapi.yaml。
# 产物：../skills/<服务名>/（单层：SKILL.md + references/，name = 服务名；本服务 media-example）。
# 别人拿到 skills/ 目录即可按服务名找到对应接口 skill，指导如何调用 API。
# openapi-to-skills 会在 -o 指定目录下再建一层 --name 子目录，故 -o 指向 skills/、
# --name 用服务名，产出正好 skills/<服务名>/（单层）。
SKILL_NAME ?= media-example
skill-gen:
	cd ../skills && openapi-to-skills \
		../media_example/swagger/openapi.yaml \
		-o . --name $(SKILL_NAME) --groupBy=path --force

# ---- 数据库 Migration（golang-migrate） ----
# migrations/ 在项目根；启动期由 main.go 经 go:embed 自动执行（database.migrate_on_start 门控）。
# 下面三个目标供手动操作：新建迁移 / 升级到最新 / 回滚一步（prod 运维或本地调试）。
# 依赖：migrate CLI（go install -tags 'mysql' github.com/golang-migrate/migrate/v4/cmd/migrate@latest）。
MYSQL_DSN ?= mysql://$(MYSQL_DSN)

.PHONY: migrate-new
# migrate-new：生成一对空 up/down 迁移文件。用法：make migrate-new NAME=add_xxx
migrate-new:
	@test -n "$(NAME)" || { echo "usage: make migrate-new NAME=add_xxx"; exit 1; }
	migrate create -ext sql -dir migrations -seq $(NAME)

.PHONY: migrate-up
# migrate-up：执行所有未应用的迁移到最新。
migrate-up:
	migrate -path migrations -database "$(MYSQL_DSN)" up

.PHONY: migrate-down
# migrate-down：回滚最近一步迁移。
migrate-down:
	migrate -path migrations -database "$(MYSQL_DSN)" down 1

# 初始化 lefthook git hooks：安装 lefthook 并写入 .git/hooks。
# lefthook.yml 已纳入版本库，团队成员 clone 后跑一次即可。
.PHONY: lefthook-install
lefthook-install:
	go install github.com/evilmartians/lefthook@latest
	lefthook install
