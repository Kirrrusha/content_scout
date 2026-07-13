GO ?= go
DOCKER_COMPOSE ?= docker compose

.PHONY: build test lint migrate-up migrate-down run-api run-bot run-tdlib run-worker docker-up docker-down

build:
	$(GO) build ./cmd/...

test:
	$(GO) test ./...

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
