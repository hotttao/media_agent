# media-cli Makefile
#
# e2e 测试:docker-compose 起服务 + 注册 smoke 账号 + 跑 e2e + 停服务。
# 符合 harness/contributing/e2e.md 规范(被测服务由 docker-compose 起;media-cli 只发 HTTP)。
#
# 用法:
#   make e2e          # 一键:起服务 + register smoke + 跑 e2e + down
#   make e2e-up       # 只起服务
#   make e2e-test     # 只跑 e2e(服务已起)
#   make e2e-down     # 只停服务

ROOT := ..

COMPOSE_FILES := \
	-f $(ROOT)/docker-compose/docker-compose.yml \
	-f $(ROOT)/media_auth/docker-compose.yml \
	-f $(ROOT)/media_materials/docker-compose.yml \
	-f $(ROOT)/media_workflow/docker-compose.yml \
	-f $(ROOT)/media_gen/docker-compose.yml

.PHONY: e2e e2e-up e2e-register-smoke e2e-test e2e-down

# 联合起 pg + 5 服务(各服务 migrate_on_start 自动跑 migration + asset_kind seed)
e2e-up:
	@echo ">> starting pg + 5 services via docker-compose..."
	cd $(ROOT) && INFRA=unit docker compose $(COMPOSE_FILES) up -d
	@echo ">> waiting 15s for services to be ready + migrate..."
	@sleep 15
	@echo ">> services up: media_auth(8001) materials(8003) workflow(8002) gen(8005)"

# 注册 smoke 测试账号(register API 下发 cookie + 建 role=user 账号)
# 已存在则忽略(幂等)
e2e-register-smoke:
	@echo ">> registering smoke test user (idempotent)..."
	@curl -s -o /dev/null -w "%{http_code}" -X POST http://127.0.0.1:8001/api/auth/register \
		-H "Content-Type: application/json" \
		-d '{"username":"smoke","password":"smoke123"}' | grep -q "200\|409" \
		&& echo ">> smoke user ready" \
		|| echo ">> WARN: register smoke failed (may already exist or service not ready)"

# 跑 e2e 测试(核心链路:素材->模板->workflow->submit->claim->complete->产物)
e2e-test:
	MEDIA_AUTH_ADDR=http://127.0.0.1:8001 \
	MEDIA_MATERIALS_ADDR=http://127.0.0.1:8003 \
	MEDIA_WORKFLOW_ADDR=http://127.0.0.1:8002 \
	MEDIA_AUTH_E2E_USER=smoke MEDIA_AUTH_E2E_PASS=smoke123 \
	go test ./e2e/ -run TestWorkflowFull -v -count=1

# 一键 e2e:起 + register + 跑 + down
e2e: e2e-up e2e-register-smoke e2e-test e2e-down

# 停服务(pg 数据保留,便于重复跑)
e2e-down:
	@echo ">> stopping services (pg data retained)..."
	cd $(ROOT) && docker compose $(COMPOSE_FILES) down
