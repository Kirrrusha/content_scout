# Telegram Summary Bot

[Русская версия](README.ru.md)

Personal Telegram summary service with Markdown export for Obsidian.

This repository is currently at `PR-010 Summary Bot UI`: project structure, configuration, HTTP health checks, PostgreSQL connection, migrations, Docker Compose, domain entities, repository interfaces, PostgreSQL repositories, Telegram bot shell, TDLib authorization state machine, folder/chat sync pipeline, user-defined source groups, message collection jobs, filtering, duplicate clustering, LLM summary generation, and summary history browsing in the bot/API.

## Architecture

- `cmd/api`: internal HTTP API. Exposes `/health` and `/ready`.
- `cmd/migrate`: lightweight SQL migration runner.
- `cmd/bot`: Telegram Bot process with owner-only shell navigation.
- `cmd/tdlib-worker`: TDLib worker shell.
- `cmd/summary-worker`: background summary worker placeholder for later PRs.
- `internal/collection`: message collection use cases for source groups.
- `internal/config`: environment-based configuration.
- `internal/domain`: core entities and enums.
- `internal/storage`: repository interfaces.
- `internal/storage/postgres`: PostgreSQL connection, migrations, repository implementations.
- `internal/sourcegroups`: source group use cases and ownership validation.
- `internal/summary/filter`: message normalization, noise filtering, advertisement/job heuristics, and filter stats.
- `internal/summary/deduplicator`: duplicate clustering by exact hash, shared URL, and Jaccard similarity.
- `internal/summary/llm`: LLM provider interfaces, OpenAI-compatible adapter, strict JSON parsing, and retry handling.
- `internal/summary/pipeline`: composed filter + deduplication processing for collected messages.
- `internal/summary`: summary generation service from collection jobs and owner-checked summary browser.
- `internal/telegram/bot`: Telegram Bot API polling, owner guard, menu routing, callback routing, cached folder/chat views, summary history UI, topic cards, and in-memory dialog state.
- `internal/telegram/tdlib`: TDLib client interface, authorization state machine, session persistence, folder/chat sync service, and unavailable native adapter placeholder.
- `migrations`: reversible SQL migrations.

The native TDLib adapter is intentionally not connected yet. Authorization and folder/chat sync logic are implemented behind interfaces and covered with fake clients, so the real adapter can be added without changing bot/API flows.

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
/groups
/group_create <name>
/group_rename <id> <name>
/group_delete <id>
/group_chats <id>
/group_add_chat <group_id> <chat_id> [priority]
/group_remove_chat <group_id> <chat_id>
/collect_group <group_id> [new|24h|3d|week|latest_n] [limit]
/summarize_collection <collection_job_id> [short|standard|detailed]
/summaries
/summary <summary_id>
/summary_topics <summary_id>
/topic <summary_id> <position>
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

Internal sync endpoints:

```text
POST   /telegram/sync
GET    /telegram/folders?telegram_user_id=...
GET    /telegram/chats?telegram_user_id=...
```

`POST /telegram/sync` uses `{"telegram_user_id": ...}`. Private chats are excluded from persistence by default. Cached chat responses include title, type, unread count, mute/archive flags, and last message id.

Internal source group endpoints:

```text
GET    /groups?telegram_user_id=...
POST   /groups
PATCH  /groups/{id}
DELETE /groups/{id}
GET    /groups/{id}/chats?telegram_user_id=...
POST   /groups/{id}/chats
DELETE /groups/{id}/chats/{chatId}
```

Group create/update bodies use `telegram_user_id`, `name`, and optional `description`. Adding a chat uses `telegram_user_id`, `chat_id`, optional `priority`, and optional `enabled`.

Internal message collection endpoint:

```text
POST   /collections/group/{id}
```

Request body:

```json
{"telegram_user_id": 123, "mode": "new", "limit": 100}
```

Supported modes are `new`, `24h`, `3d`, `week`, and `latest_n`. Collection jobs store fetched messages but intentionally do not advance `read_positions`; that happens only after a later successful summary.

Filtering and deduplication currently run as pure Go services over collected messages:

- combines message text and caption;
- normalizes whitespace and line endings;
- removes common Telegram footers;
- extracts URLs;
- removes empty, emoji-only, too-short, ad-like, and job-like messages according to rules;
- groups duplicates by content hash, shared URL, and token Jaccard similarity.

Internal summary endpoint:

```text
POST   /summaries/from-collection/{id}
GET    /summaries?telegram_user_id=...&limit=...
GET    /summaries/{id}?telegram_user_id=...
GET    /summaries/{id}/topics?telegram_user_id=...
```

Request body:

```json
{"telegram_user_id": 123, "format": "standard"}
```

Summary generation uses the collected messages, filter/deduplication pipeline, an OpenAI-compatible chat completions provider, strict JSON validation, retry handling, and persists `summary_jobs`, `summaries`, and `summary_topics`. Summary browsing is owner-checked and exposes the latest summaries, full markdown for a single summary, and ordered topic cards for bot navigation.

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
- `message_collection_jobs`
- `collected_messages`
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

`PR-011 — Article conversion` should convert saved summaries/topics into article drafts.
