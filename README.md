# Telegram Summary Bot

[Русская версия](README.ru.md)

Personal Telegram summary service with Markdown export for Obsidian.

## Architecture

- `cmd/api`: internal HTTP API. Exposes `/health` and `/ready`.
- `cmd/migrate`: lightweight SQL migration runner.
- `cmd/bot`: Telegram Bot process with owner-only shell navigation.
- `cmd/tdlib-worker`: TDLib worker entrypoint.
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
- `internal/telegram/tdlib`: TDLib client interface, optional native `tdjson` adapter, authorization state machine, session persistence, folder/chat sync service, chat/message mapping, and unavailable fallback for non-TDLib builds.
- `migrations`: reversible SQL migrations.

The native TDLib adapter is enabled in Docker builds and in local builds compiled with `-tags tdlib` and CGO. Plain local builds keep using the unavailable fallback, so unit tests and non-TDLib development do not require `libtdjson`.

## Configuration

Create a local env file:

```sh
cp .env.example .env
```

For local non-Docker runs, set `DATABASE_URL` to a host database, for example:

```sh
export DATABASE_URL='postgres://postgres:postgres@localhost:5432/telegram_summary?sslmode=disable'
```

Set `SERVICE_TOKEN` for the internal API. Every endpoint except `GET /health` and `GET /ready` requires:

```http
Authorization: Bearer <token>
```

### Environment Variables

| Variable | Required | Default | Description |
|---|---:|---|---|
| `APP_ENV` | no | `development` | Application environment label used by services and logs. |
| `HTTP_ADDR` | no | `:8080` | API listen address. |
| `DATABASE_URL` | yes | local PostgreSQL URL in code, Docker URL in `.env.example` | PostgreSQL connection string. |
| `SERVICE_TOKEN` | yes for `cmd/api` | empty | Bearer token for all internal API endpoints except `/health` and `/ready`. |
| `LOG_FORMAT` | no | `json` | Log format: `json` for Docker/production-like runs or `text` for local development. |
| `LOG_LEVEL` | no | `info` | Minimum log level: `debug`, `info`, `warn`, or `error`. |
| `LOG_DIR` | no | `./data/logs` locally, `/data/logs` in Docker | Directory for local rotated log files. Empty disables file logging. |
| `LOG_RETENTION` | no | `24h` | Deletes log files older than this duration. |
| `LOG_ROTATION_INTERVAL` | no | `1h` | File rotation and cleanup tick interval. |
| `WORKER_ID` | no | hostname-based | Stable id written to job locks and logs by worker processes. |
| `TELEGRAM_BOT_TOKEN` | yes for `cmd/bot` | empty | Telegram Bot API token. If empty, the bot exits idle. |
| `TELEGRAM_OWNER_ID` | yes for bot/API actions | `0` | Telegram user id allowed to control the bot and API flows. |
| `TELEGRAM_API_ID` | yes for native TDLib | `0` | Telegram API id from my.telegram.org. |
| `TELEGRAM_API_HASH` | yes for native TDLib | empty | Telegram API hash from my.telegram.org. |
| `TDLIB_DATABASE_DIR` | no | `./data/tdlib` locally, `/data/tdlib` in Docker | TDLib session/database directory. Persist this directory. |
| `TDLIB_GIT_REF` | no | `master` | TDLib source branch/tag/commit used by Docker builds. |
| `TDLIB_INTEGRATION_SESSION_DIR` | no | temp dir | Session directory for optional native TDLib integration tests. |
| `LLM_PROVIDER` | no | `openai` | LLM provider label. The current adapter is OpenAI-compatible. |
| `LLM_BASE_URL` | depends on provider | empty | OpenAI-compatible chat completions base URL. |
| `LLM_API_KEY` | yes for summary/article generation | empty | LLM API key. Do not log it. |
| `LLM_MODEL` | yes for summary/article generation | empty | Model name for summary and article generation. |
| `ENCRYPTION_KEY` | reserved | empty | Reserved for future encrypted secret storage. |
| `EXPORT_DIR` | no | `./data/exports` locally, `/data/exports` in Docker | Directory for Markdown exports. |
| `OBSIDIAN_REST_URL` | no | empty | Obsidian Local REST API base URL. |
| `OBSIDIAN_API_KEY` | no | empty | Obsidian Local REST API key. Enables REST export when set. |
| `OBSIDIAN_INSECURE_SKIP_VERIFY` | no | `false` | Allow self-signed Obsidian HTTPS certificate. |

## Commands

| Command | Description |
|---|---|
| `make build` | Build all Go command binaries with the default non-TDLib local configuration. |
| `make build-tdlib` | Build `api`, `bot`, and `tdlib-worker` with `-tags tdlib` and CGO enabled. Requires local `libtdjson`. |
| `make test` | Run the full default Go test suite. |
| `make test-tdlib-nocgo` | Run TDLib package tests with the `tdlib` tag but CGO disabled, verifying the fallback path. |
| `make test-tdlib-integration` | Run optional native TDLib integration tests. Requires `libtdjson`, Telegram API credentials, and a session directory. |
| `make lint` | Run `go vet ./...`. |
| `make migrate-up` | Apply SQL migrations from `migrations`. |
| `make migrate-down` | Roll back SQL migrations from `migrations`. |
| `make run-api` | Run the internal HTTP API locally. |
| `make run-bot` | Run the Telegram bot locally. |
| `make run-tdlib` | Run the TDLib worker entrypoint locally. |
| `make run-worker` | Run the summary worker locally. |
| `make docker-up` | Create `.env` if missing and start Docker Compose with rebuilds. |
| `make docker-down` | Stop and remove Docker Compose containers. |

The bot exits idle when `TELEGRAM_BOT_TOKEN` or `TELEGRAM_OWNER_ID` is not configured. With both values set, it starts Telegram long polling and only responds to the configured owner.

Bot commands currently available:

| Command | Description |
|---|---|
| `/start` | Open the main bot menu. |
| `/connect` | Start or resume TDLib account authorization. |
| `/phone <number>` | Submit the phone number requested by TDLib. |
| `/code <code>` | Submit the Telegram login confirmation code. |
| `/password <2fa password>` | Submit the 2FA password when the account requires it. |
| `/session` | Show current TDLib session and authorization state. |
| `/delete_session` | Log out and delete the stored TDLib session. |
| `/folders` | Show cached Telegram folders. |
| `/chats` | Show cached Telegram chats. |
| `/sync` | Sync folders and chats from Telegram through TDLib. |
| `/groups` | List configured source groups. |
| `/group_create <name>` | Create a source group. |
| `/group_rename <id> <name>` | Rename an existing source group. |
| `/group_delete <id>` | Delete a source group. |
| `/group_chats <id>` | List chats attached to a source group. |
| `/group_add_chat <group_id> <chat_id> [priority]` | Add a Telegram chat to a source group. |
| `/group_remove_chat <group_id> <chat_id>` | Remove a chat from a source group. |
| `/collect_group <group_id> [new\|24h\|3d\|week\|latest_n] [limit]` | Collect messages from all enabled chats in a group. |
| `/summarize_collection <collection_job_id> [short\|standard\|detailed]` | Generate a summary from a collection job. |
| `/summaries` | Show recent summaries. |
| `/summary <summary_id>` | Show one summary in full. |
| `/summary_topics <summary_id>` | Show topics extracted from a summary. |
| `/topic <summary_id> <position>` | Show a specific summary topic. |
| `/article_from_summary <summary_id> [analysis\|guide\|educational\|outline\|telegram_post]` | Convert a full summary into an article draft. |
| `/article_from_topic <summary_id> <position> [analysis\|guide\|educational\|outline\|telegram_post]` | Convert one summary topic into an article draft. |
| `/articles` | Show recent article drafts. |
| `/article <article_id>` | Show one article draft. |
| `/article_title <article_id> <new title>` | Update an article title. |
| `/article_tags <article_id> tag1,tag2` | Replace article tags. |
| `/export_article <article_id>` | Export an article draft to Markdown and optional Obsidian REST. |
| `/export_summary <summary_id>` | Export a summary to Markdown and optional Obsidian REST. |
| `/settings` | Open settings and account/session controls. |
| `/schedules` | List configured schedules. |
| `/schedule_create <group_id> <HH:MM> [timezone] [export:true\|false]` | Create a daily schedule for a source group. |
| `/schedule <id>` | Show one schedule and its latest runs. |
| `/schedule_enable <id>` | Enable a schedule. |
| `/schedule_disable <id>` | Disable a schedule. |
| `/schedule_delete <id>` | Delete a schedule. |
| `/schedule_run <id>` | Queue a schedule for immediate execution. |

Authorization inputs are routed through the TDLib state machine. Phone numbers, confirmation codes, and 2FA passwords are not logged or stored by the application.

Native TDLib local builds require `libtdjson` to be installed and discoverable by the system linker:

```sh
CGO_ENABLED=1 go build -tags tdlib ./cmd/api ./cmd/bot ./cmd/tdlib-worker
```

The optional native integration test is intentionally separate from unit tests:

```sh
export TELEGRAM_API_ID=123
export TELEGRAM_API_HASH=...
export TDLIB_INTEGRATION_SESSION_DIR=./data/tdlib-integration
make test-tdlib-integration
```

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

Internal schedule endpoints:

```text
GET    /schedules?telegram_user_id=...
POST   /schedules
GET    /schedules/{id}?telegram_user_id=...
PATCH  /schedules/{id}
DELETE /schedules/{id}
POST   /schedules/{id}/enable
POST   /schedules/{id}/disable
POST   /schedules/{id}/run
GET    /schedules/{id}/runs?telegram_user_id=...&limit=...
```

Create/update body:

```json
{
  "telegram_user_id": 123,
  "source_group_id": 12,
  "time": "09:00",
  "timezone": "Europe/Amsterdam",
  "quiet_hours_start": "23:00",
  "quiet_hours_end": "07:00",
  "summary_type": "standard",
  "export_enabled": true,
  "enabled": true
}
```

Delete/enable/disable/run bodies use:

```json
{"telegram_user_id": 123}
```

## Job Queue

Background work is coordinated through the PostgreSQL `jobs` table. Workers claim due jobs with `FOR UPDATE SKIP LOCKED`, set `locked_by` and `lease_expires_at`, and then mark the job as `completed`, `retry_wait`, or `dead`.

The queue stores:

- `type`, `status`, and JSON `payload`;
- `attempt`, `max_attempts`, and `available_at`;
- `locked_at`, `locked_by`, and `lease_expires_at`;
- `last_error`, `started_at`, `finished_at`;
- optional `deduplication_key`.

Supported job types are:

```text
telegram_sync
message_collection
summary_generation
article_generation
obsidian_export
scheduled_pipeline
```

`cmd/summary-worker` currently enqueues due schedules as `scheduled_pipeline` jobs and then processes queued jobs. Scheduled jobs use a per-schedule/per-local-day deduplication key, so repeated polling does not create duplicates. Temporary errors are retried with exponential backoff; permanent configuration/input errors go to `dead`.

Multiple workers can run safely:

```sh
docker compose --profile worker up --scale summary-worker=3
```

## Local Logs

Services use `log/slog` and write to stdout plus hourly files under `LOG_DIR`.

```text
data/logs/
├── api-2026-07-13-18.log
├── api-current.log
├── bot-2026-07-13-18.log
├── summary-worker-2026-07-13-18.log
└── tdlib-worker-2026-07-13-18.log
```

Files older than `LOG_RETENTION` are removed by a lightweight cleanup loop in each process. Sensitive values such as authorization codes, 2FA passwords, API keys, service tokens, and full private message text should not be logged.

Examples:

```sh
tail -f data/logs/summary-worker-current.log
grep '"level":"ERROR"' data/logs/*.log
```

## Docker

```sh
make docker-up
```

The `api`, `bot`, and `tdlib-worker` Docker images build TDLib from the official `tdlib/td` repository and compile the Go binaries with `-tags tdlib`. You can pin the TDLib source revision with the `TDLIB_GIT_REF` build arg if needed.

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
- `jobs`

Integration tests use `TEST_DATABASE_URL`. If it is not set, PostgreSQL integration tests are skipped.

```sh
export TEST_DATABASE_URL='postgres://postgres:postgres@localhost:5432/telegram_summary_test?sslmode=disable'
go test ./internal/storage/postgres
```

## Security Notes

- `.env` is ignored by git.
- Sensitive Telegram and LLM fields are config-only at this stage.
- Internal API endpoints require `SERVICE_TOKEN` except `/health` and `/ready`.
- Logs avoid message content and secrets.
- Docker services run as a non-root user where application containers are used.

## Next PR

The planned PR sequence from the updated roadmap is complete through `PR-019`.
