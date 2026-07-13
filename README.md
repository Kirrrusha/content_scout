# Telegram Summary Bot

Personal Telegram summary service with Markdown export for Obsidian.

This repository is currently at `PR-004 TDLib authorization`: project structure, configuration, HTTP health checks, PostgreSQL connection, migrations, Docker Compose, domain entities, repository interfaces, PostgreSQL repositories, Telegram bot shell, and TDLib authorization state machine.

## Architecture

- `cmd/api`: internal HTTP API. Exposes `/health` and `/ready`.
- `cmd/migrate`: lightweight SQL migration runner.
- `cmd/bot`: Telegram Bot process with owner-only shell navigation.
- `cmd/tdlib-worker`: TDLib worker shell.
- `cmd/summary-worker`: background summary worker placeholder for later PRs.
- `internal/config`: environment-based configuration.
- `internal/domain`: core entities and enums.
- `internal/storage`: repository interfaces.
- `internal/storage/postgres`: PostgreSQL connection, migrations, repository implementations.
- `internal/telegram/bot`: Telegram Bot API polling, owner guard, menu routing, callback routing, and in-memory dialog state.
- `internal/telegram/tdlib`: TDLib client interface, authorization state machine, session persistence, and unavailable native adapter placeholder.
- `migrations`: reversible SQL migrations.

The native TDLib adapter is intentionally not connected yet. Authorization logic is implemented behind an interface and covered with fake clients, so the real adapter can be added without changing bot/API flows.

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

The bot exits idle when `TELEGRAM_BOT_TOKEN` or `TELEGRAM_OWNER_ID` is not configured. With both values set, it starts Telegram long polling and only responds to the configured owner.

Bot commands currently available:

```text
/start
/connect
/phone <number>
/code <code>
/password <2fa password>
/session
/delete_session
/folders
/chats
/sync
/settings
```

Authorization inputs are routed through the TDLib state machine. Phone numbers, confirmation codes, and 2FA passwords are not logged or stored by the application.

Internal authorization endpoints:

```text
GET    /telegram/auth/status?telegram_user_id=...
POST   /telegram/auth/start
POST   /telegram/auth/phone
POST   /telegram/auth/code
POST   /telegram/auth/password
DELETE /telegram/session
```

Request bodies use `telegram_user_id` plus the relevant field: `phone`, `code`, or `password`.

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

The migrations create:

- `schema_migrations`
- `users`
- `telegram_sessions`
- `telegram_folders`
- `telegram_chats`
- `source_groups`
- `source_group_chats`
- `read_positions`
- `summary_jobs`
- `summaries`
- `summary_topics`
- `articles`
- `article_sources`
- `obsidian_exports`

Integration tests use `TEST_DATABASE_URL`. If it is not set, PostgreSQL integration tests are skipped.

```sh
export TEST_DATABASE_URL='postgres://postgres:postgres@localhost:5432/telegram_summary_test?sslmode=disable'
go test ./internal/storage/postgres
```

## Security Notes

- `.env` is ignored by git.
- Sensitive Telegram and LLM fields are config-only at this stage.
- Logs avoid message content and secrets.
- Docker services run as a non-root user where application containers are used.

## Next PR

`PR-005 — Telegram folders and chats` should connect the native TDLib adapter enough to sync folders/chats, store unread counts, and show them through the bot.
