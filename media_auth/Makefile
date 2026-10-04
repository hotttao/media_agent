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

# 业务 IDL：../idl/media_auth/ 下所有 .proto（相对 IDL 根，去掉 ../idl/ 前缀）。
# 注意：hz update 对 register.go 的 INSERT_POINT 累积有顺序敏感性（v0.9.1），
# token/auth/oauth must precede suffix-containing api_token/mcp_oauth etc.
# hz v0.9.1 checks registration by substring; suffix-first causes false duplicates.
BIZ_PROTOS := media_auth/token.proto media_auth/auth.proto media_auth/oauth.proto \
	$(filter-out media_auth/token.proto media_auth/auth.proto media_auth/oauth.proto,$(shell find ../idl/media_auth -type f -name '*.proto' | sed 's|^\.\./idl/||' | sort))

# 参与 OpenAPI 文档生成的正式 service proto：排除 token.proto（internal mTLS，不挂网关/不出 OpenAPI）。
# 约定：正式 service proto 必须同时挂 api.* 与 google.api.http 两条注解（见 idl.md §2）。
SWAGGER_PROTOS := $(shell find ../idl/media_auth -type f -name '*.proto' \
		-not -name 'token.proto' -not -name 'platform_token.proto' | sed 's|^\.\./idl/||')

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

# ---- GORM Gen（schema 变更后重生成 query，🔴 禁改 query/） ----
# 需连真实 pg（schema 由 migrations 定义）。DSN 从环境变量读。
# 流程：加 migration → 执行迁移（make migrate-up）→ 更新 tools/gormgen model 列表 → make gorm-gen
.PHONY: gorm-gen
gorm-gen:
	go run tools/gormgen/main.go

.PHONY: swagger-gen
# swagger-gen：用 protoc-gen-openapi（gnostic）从本服务业务 IDL 生成 OpenAPI v3 文档。
# 依赖：protoc-gen-openapi（go install github.com/google/gnostic/cmd/protoc-gen-openapi@latest）。
# 仅处理正式 service proto（SWAGGER_PROTOS，排除 token.proto）。每个 RPC 必须挂
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
# 产物：../skills/<服务名>/（单层：SKILL.md + references/，name = 服务名；本服务 media-auth）。
# 别人拿到 skills/ 目录即可按服务名找到对应接口 skill，指导如何调用 API。
# openapi-to-skills 会在 -o 指定目录下再建一层 --name 子目录，故 -o 指向 skills/、
# --name 用服务名，产出正好 skills/<服务名>/（单层）。
SKILL_NAME ?= media-auth
skill-gen:
	cd ../skills && openapi-to-skills \
		../media_auth/swagger/openapi.yaml \
		-o . --name $(SKILL_NAME) --groupBy=path --force

# ---- 数据库 Migration（golang-migrate，postgres） ----
# migrations/ 在项目根；启动期由 main.go 运行时读磁盘自动执行（database.migrate_on_start 门控）。
# 下面三个目标供手动操作：新建迁移 / 升级到最新 / 回滚一步（prod 运维或本地调试）。
# 依赖：migrate CLI（go install -tags 'postgres' github.com/golang-migrate/migrate/v4/cmd/migrate@latest）。
# DSN 形如 postgres://user:pass@host:5432/db?sslmode=disable
PG_DSN ?= $(MEDIA_AUTH_DB_DSN)

.PHONY: migrate-new
# migrate-new：生成一对空 up/down 迁移文件。用法：make migrate-new NAME=add_xxx
migrate-new:
	@test -n "$(NAME)" || { echo "usage: make migrate-new NAME=add_xxx"; exit 1; }
	migrate create -ext sql -dir migrations -seq $(NAME)

.PHONY: migrate-up
# migrate-up：执行所有未应用的迁移到最新。
migrate-up:
	migrate -path migrations -database "$(PG_DSN)" up

.PHONY: migrate-down
# migrate-down：回滚最近一步迁移。
migrate-down:
	migrate -path migrations -database "$(PG_DSN)" down 1

# 初始化 lefthook git hooks：安装 lefthook 并写入 .git/hooks。
# lefthook.yml 已纳入版本库，团队成员 clone 后跑一次即可。
.PHONY: lefthook-install
lefthook-install:
	go install github.com/evilmartians/lefthook@latest
	lefthook install
