.PHONY: all dev stop deps migrate migrate-down generate build build-api build-worker build-admin-create build-migrate test test-race lint run-api run-worker admin-create clean require-database-url require-runtime-env require-admin-password

GO ?= go
DOCKER_COMPOSE ?= docker compose

CDK_DATABASE_URL ?=
CDK_REDIS_URL ?=
ADMIN_USERNAME ?= admin

all: deps generate build

dev:
	$(DOCKER_COMPOSE) up -d postgres redis

stop:
	$(DOCKER_COMPOSE) down

deps:
	$(GO) mod download
	$(GO) mod tidy

migrate: require-database-url
	CDK_DATABASE_URL="$(CDK_DATABASE_URL)" $(GO) run ./cmd/migrate up

migrate-down: require-database-url
	CDK_DATABASE_URL="$(CDK_DATABASE_URL)" $(GO) run ./cmd/migrate down

generate:
	$(GO) run github.com/sqlc-dev/sqlc/cmd/sqlc generate

build: build-api build-worker build-admin-create build-migrate

build-api:
	$(GO) build -o bin/api ./cmd/api

build-worker:
	$(GO) build -o bin/worker ./cmd/worker

build-admin-create:
	$(GO) build -o bin/admin-create ./cmd/admin-create

build-migrate:
	$(GO) build -o bin/migrate ./cmd/migrate

test:
	$(GO) test ./...

test-race:
	$(GO) test -race ./...

lint:
	$(GO) vet ./...
	$(GO) run github.com/pressly/goose/v3/cmd/goose --version || true

run-api: require-runtime-env build-api
	CDK_DATABASE_URL="$(CDK_DATABASE_URL)" CDK_REDIS_URL="$(CDK_REDIS_URL)" ./bin/api

run-worker: require-runtime-env build-worker
	CDK_DATABASE_URL="$(CDK_DATABASE_URL)" CDK_REDIS_URL="$(CDK_REDIS_URL)" ./bin/worker

admin-create: require-database-url require-admin-password build-admin-create
	CDK_DATABASE_URL="$(CDK_DATABASE_URL)" ADMIN_PASSWORD="$(ADMIN_PASSWORD)" ./bin/admin-create "$(ADMIN_USERNAME)"

require-database-url:
	@test -n "$(CDK_DATABASE_URL)" || (echo "CDK_DATABASE_URL is required" >&2; exit 1)

require-runtime-env: require-database-url
	@test -n "$(CDK_REDIS_URL)" || (echo "CDK_REDIS_URL is required" >&2; exit 1)

require-admin-password:
	@test -n "$(ADMIN_PASSWORD)" || (echo "ADMIN_PASSWORD is required" >&2; exit 1)

clean:
	rm -rf bin/
