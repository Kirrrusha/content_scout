GO ?= go
DOCKER_COMPOSE ?= docker compose

.PHONY: build build-tdlib test test-tdlib-nocgo test-tdlib-integration lint migrate-up migrate-down run-api run-bot run-tdlib run-worker docker-up docker-down

build:
	$(GO) build ./cmd/...

build-tdlib:
	CGO_ENABLED=1 $(GO) build -tags tdlib ./cmd/api ./cmd/bot ./cmd/tdlib-worker

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
	$(GO) run ./cmd/api

run-bot:
	$(GO) run ./cmd/bot

run-tdlib:
	$(GO) run ./cmd/tdlib-worker

run-worker:
	$(GO) run ./cmd/summary-worker

docker-up:
	cp -n .env.example .env || true
	$(DOCKER_COMPOSE) up --build

docker-down:
	$(DOCKER_COMPOSE) down
