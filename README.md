# Telegram Summary Bot

Personal Telegram summary service with Markdown export for Obsidian.

This repository is currently at `PR-001 Bootstrap`: project structure, configuration, HTTP health checks, PostgreSQL connection, migrations, Docker Compose, Makefile, CI, and the first `users` repository.

## Architecture

- `cmd/api`: internal HTTP API. Exposes `/health` and `/ready`.
- `cmd/migrate`: lightweight SQL migration runner.
- `cmd/bot`: Telegram Bot process placeholder for PR-003.
- `cmd/tdlib-worker`: TDLib worker placeholder for PR-004.
- `cmd/summary-worker`: background summary worker placeholder for later PRs.
- `internal/config`: environment-based configuration.
- `internal/storage/postgres`: PostgreSQL connection, migrations, repositories.
- `migrations`: reversible SQL migrations.

TDLib and LLM integrations are intentionally not connected in this bootstrap step.

## Configuration

Create a local env file:

```sh
cp .env.example .env
```

For local non-Docker runs, set `DATABASE_URL` to a host database, for example:

```sh
export DATABASE_URL='postgres://postgres:postgres@localhost:5432/telegram_summary?sslmode=disable'
```

## Commands

```sh
make build
make test
make lint
make migrate-up
make migrate-down
make run-api
make run-bot
make run-tdlib
make run-worker
make docker-up
make docker-down
```

## Docker

```sh
make docker-up
```

The API is available at:

```text
http://localhost:8080/health
http://localhost:8080/ready
```

## Database

The first migration creates:

- `schema_migrations`
- `users`

Integration tests use `TEST_DATABASE_URL`. If it is not set, PostgreSQL integration tests are skipped.

## Security Notes

- `.env` is ignored by git.
- Sensitive Telegram and LLM fields are config-only at this stage.
- Logs avoid message content and secrets.
- Docker services run as a non-root user where application containers are used.

## Next PR

`PR-002 — Domain and repositories` should add the remaining domain entities, repository interfaces, PostgreSQL implementations, and focused tests for persistence behavior.
