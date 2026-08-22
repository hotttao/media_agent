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

BIZ_PROTOS := $(shell find ../idl/media_agent_gateway -type f -name '*.proto' | sed 's|^\.\./idl/||')

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

.PHONY: migrate-agent-history verify-agent-history cleanup-agent-history
migrate-agent-history:
	go run ./cmd/agent-history-migrate --mode import

verify-agent-history:
	go run ./cmd/agent-history-migrate --mode verify

cleanup-agent-history:
	go run ./cmd/agent-history-migrate --mode cleanup --confirm-drop-legacy-history

.PHONY: lefthook-install
lefthook-install:
	go install github.com/evilmartians/lefthook@latest
	lefthook install
