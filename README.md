# Telegram Summary Bot

[Русская версия](README.ru.md)

Personal Telegram summary service with Markdown export for Obsidian.

This repository is currently at `PR-014 Obsidian REST integration`: project structure, configuration, HTTP health checks, PostgreSQL connection, migrations, Docker Compose, domain entities, repository interfaces, PostgreSQL repositories, Telegram bot shell, TDLib authorization state machine, folder/chat sync pipeline, user-defined source groups, message collection jobs, filtering, duplicate clustering, LLM summary generation, summary history browsing, article draft conversion, Markdown export for Obsidian, scheduled summary runs, and optional Obsidian Local REST API note writes.

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
- `internal/article`: article conversion use cases, draft persistence, source link capture, slug generation, and title/tag metadata updates.
- `internal/obsidian`: Markdown rendering, YAML frontmatter, safe filenames, SHA-256 deduplication, export file persistence, and optional Obsidian Local REST API create/update with backups.
- `internal/scheduler`: enabled schedule polling, timezone-aware daily due checks, quiet hours, and collection -> summary -> optional export orchestration.
- `internal/telegram/bot`: Telegram Bot API polling, owner guard, menu routing, callback routing, cached folder/chat views, summary history UI, topic cards, article draft actions, and in-memory dialog state.
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
/article_from_summary <summary_id> [analysis|guide|educational|outline|telegram_post]
/article_from_topic <summary_id> <position> [analysis|guide|educational|outline|telegram_post]
/articles
/article <article_id>
/article_title <article_id> <new title>
/article_tags <article_id> tag1,tag2
/export_article <article_id>
/export_summary <summary_id>
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

Internal article endpoints:

```text
POST   /articles/from-summary/{id}
POST   /articles/from-summary/{id}/topics/{position}
GET    /articles?telegram_user_id=...&limit=...
GET    /articles/{id}?telegram_user_id=...
PATCH  /articles/{id}
```

Conversion request body:

```json
{"telegram_user_id": 123, "type": "analysis", "title": "Optional title", "tags": ["telegram", "ai"]}
```

Article conversion uses the same OpenAI-compatible provider with a dedicated JSON prompt, saves drafts to `articles`, stores source links in `article_sources`, generates unique slugs, and supports owner-checked title/tag updates.

Internal Obsidian export endpoints:

```text
POST   /exports/articles/{id}
POST   /exports/summaries/{id}
```

Request body:

```json
{"telegram_user_id": 123}
```

Markdown exports are written under `EXPORT_DIR`, include YAML frontmatter, use safe `.md` filenames, preserve Telegram source links for article drafts, calculate SHA-256 content hashes, and reuse existing `obsidian_exports` records for identical content. The bot sends the generated Markdown as a Telegram document.

If `OBSIDIAN_API_KEY` is set, exports are also written directly to Obsidian through the [Local REST API plugin](https://github.com/coddingtonbear/obsidian-local-rest-api). Configure `OBSIDIAN_REST_URL` for the API base URL, and set `OBSIDIAN_INSECURE_SKIP_VERIFY=true` when using the plugin's self-signed HTTPS certificate. Existing notes are backed up to `*.backup-YYYYMMDD-HHMMSS.md` before update.

Scheduled summaries are stored in `summary_schedules` and executed by `cmd/summary-worker`. The MVP supports daily schedule strings in `HH:MM`, `daily@HH:MM`, or `@daily` format, IANA timezones, quiet-hour windows, summary format, and optional export to Obsidian. Each attempt is recorded in `schedule_runs`.

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
- `summary_schedules`
- `schedule_runs`

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

The planned PR sequence from the initial roadmap is complete through `PR-014`. A natural next step is hardening: native TDLib adapter, service-token auth for the internal API, and observability.
