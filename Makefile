GO ?= go
DOCKER_COMPOSE ?= docker compose

BREW_PREFIX ?= $(shell brew --prefix 2>/dev/null)
TDLIB_CGO_LDFLAGS ?= $(if $(BREW_PREFIX),-L$(BREW_PREFIX)/lib,)
LOCAL_DATABASE_URL ?= postgres://postgres:postgres@127.0.0.1:5432/telegram_summary?sslmode=disable
LOCAL_TDLIB_DATABASE_DIR ?= ./data/tdlib
LOAD_ENV = set -a; [ ! -f .env ] || . ./.env; set +a

.PHONY: build build-tdlib test test-tdlib-nocgo test-tdlib-integration lint migrate-up migrate-down run-api run-bot run-tdlib run-worker docker-up docker-down

build:
	$(GO) build ./cmd/...

build-tdlib:
	CGO_ENABLED=1 CGO_LDFLAGS="$(TDLIB_CGO_LDFLAGS)" $(GO) build -tags tdlib ./cmd/api ./cmd/tdlib-worker
	CGO_ENABLED=0 $(GO) build ./cmd/bot

test:
	$(GO) test ./...

test-tdlib-nocgo:
	CGO_ENABLED=0 $(GO) test -tags tdlib ./internal/telegram/tdlib

test-tdlib-integration:
	CGO_ENABLED=1 $(GO) test -tags 'tdlib integration' ./internal/telegram/tdlib

lint:
	$(GO) vet ./...

migrate-up:
	$(GO) run ./cmd/migrate -direction up -dir migrations

migrate-down:
	$(GO) run ./cmd/migrate -direction down -dir migrations

run-api:
	$(LOAD_ENV); DATABASE_URL="$(LOCAL_DATABASE_URL)" TDLIB_DATABASE_DIR="$(LOCAL_TDLIB_DATABASE_DIR)" CGO_ENABLED=1 CGO_LDFLAGS="$(TDLIB_CGO_LDFLAGS)" $(GO) run -tags tdlib ./cmd/api

run-bot:
	$(LOAD_ENV); DATABASE_URL="$(LOCAL_DATABASE_URL)" CGO_ENABLED=0 $(GO) run ./cmd/bot

run-tdlib:
	$(LOAD_ENV); TDLIB_DATABASE_DIR="$(LOCAL_TDLIB_DATABASE_DIR)" CGO_ENABLED=1 CGO_LDFLAGS="$(TDLIB_CGO_LDFLAGS)" $(GO) run -tags tdlib ./cmd/tdlib-worker

run-worker:
	$(LOAD_ENV); DATABASE_URL="$(LOCAL_DATABASE_URL)" $(GO) run ./cmd/summary-worker

docker-up:
	cp -n .env.example .env || true
	$(DOCKER_COMPOSE) up --build

docker-down:
	$(DOCKER_COMPOSE) down
