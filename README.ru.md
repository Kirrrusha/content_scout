# Telegram Summary Bot

[English version](README.md)

Персональный сервис для сводок из Telegram с экспортом Markdown-файлов в Obsidian.

Текущее состояние репозитория: `PR-015 Native TDLib Adapter`. Уже есть структура проекта, конфигурация, HTTP health checks, подключение PostgreSQL, миграции, Docker Compose, доменные сущности, repository interfaces, PostgreSQL repositories, shell Telegram-бота, native TDLib JSON adapter, state machine авторизации TDLib, pipeline синхронизации папок/чатов, пользовательские группы источников, jobs сбора сообщений, фильтрация, группировка дублей, генерация summary через LLM, просмотр истории summary, конвертация summary/topics в черновики статей, Markdown export для Obsidian, scheduled summary runs и optional Obsidian Local REST API note writes.

## Архитектура

- `cmd/api`: внутренний HTTP API. Доступны `/health` и `/ready`.
- `internal/collection`: сценарии сбора сообщений для групп источников.
- `cmd/migrate`: легкий runner SQL-миграций.
- `cmd/bot`: процесс Telegram-бота с owner-only навигацией.
- `cmd/tdlib-worker`: entrypoint TDLib worker.
- `cmd/summary-worker`: placeholder фонового summary worker для следующих PR.
- `internal/config`: конфигурация через переменные окружения.
- `internal/domain`: доменные сущности и enum'ы.
- `internal/sourcegroups`: сценарии source groups и проверка владения.
- `internal/summary/filter`: нормализация сообщений, фильтрация шума, эвристики рекламы/вакансий и статистика фильтрации.
- `internal/summary/deduplicator`: группировка дублей по exact hash, общей ссылке и Jaccard similarity.
- `internal/summary/llm`: provider interfaces, OpenAI-compatible adapter, строгий JSON parsing и retry handling.
- `internal/summary/pipeline`: общий filter + deduplication pipeline для collected messages.
- `internal/summary`: сервис генерации summary из collection jobs и owner-checked browser для истории summary.
- `internal/article`: сценарии конвертации статей, сохранение draft, source links, генерация slug и обновление title/tags.
- `internal/obsidian`: Markdown renderer, YAML frontmatter, безопасные имена файлов, SHA-256 deduplication, сохранение export files и optional Obsidian Local REST API create/update с backups.
- `internal/scheduler`: polling enabled schedules, timezone-aware daily due checks, quiet hours и orchestration collection -> summary -> optional export.
- `internal/storage`: repository interfaces.
- `internal/storage/postgres`: PostgreSQL connection, миграции и реализации repositories.
- `internal/telegram/bot`: Telegram Bot API polling, owner guard, меню, callback routing, просмотр кэша папок/чатов, UI истории summary, карточки тем, действия с черновиками статей и in-memory dialog state.
- `internal/telegram/tdlib`: TDLib client interface, optional native `tdjson` adapter, state machine авторизации, сохранение сессии, sync service для папок/чатов, маппинг чатов/сообщений и unavailable fallback для сборок без TDLib.
- `migrations`: обратимые SQL-миграции.

Native TDLib adapter включён в Docker-сборках и в локальных сборках с `-tags tdlib` и CGO. Обычные локальные сборки используют unavailable fallback, поэтому unit-тесты и разработка без TDLib не требуют `libtdjson`.

## Конфигурация

Создайте локальный env-файл:

```sh
cp .env.example .env
```

Для запуска без Docker укажите `DATABASE_URL` на локальную базу:

```sh
export DATABASE_URL='postgres://postgres:postgres@localhost:5432/telegram_summary?sslmode=disable'
```

## Команды

```sh
make build
make build-tdlib
make test
make test-tdlib-nocgo
make test-tdlib-integration
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

Бот уходит в idle-режим, если не заданы `TELEGRAM_BOT_TOKEN` или `TELEGRAM_OWNER_ID`. Если оба значения заданы, бот запускает Telegram long polling и отвечает только настроенному владельцу.

Текущие команды бота:

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
/article_title <article_id> <новое название>
/article_tags <article_id> tag1,tag2
/export_article <article_id>
/export_summary <summary_id>
/settings
```

Данные авторизации проходят через TDLib state machine. Номера телефонов, коды подтверждения и 2FA-пароли не логируются и не сохраняются приложением.

Локальные native TDLib-сборки требуют установленный `libtdjson`, доступный системному linker:

```sh
CGO_ENABLED=1 go build -tags tdlib ./cmd/api ./cmd/bot ./cmd/tdlib-worker
```

Опциональный native integration test намеренно отделён от unit-тестов:

```sh
export TELEGRAM_API_ID=123
export TELEGRAM_API_HASH=...
export TDLIB_INTEGRATION_SESSION_DIR=./data/tdlib-integration
make test-tdlib-integration
```

Внутренние endpoints авторизации:

```text
GET    /telegram/auth/status?telegram_user_id=...
POST   /telegram/auth/start
POST   /telegram/auth/phone
POST   /telegram/auth/code
POST   /telegram/auth/password
DELETE /telegram/session
```

Тела запросов используют `telegram_user_id` и соответствующее поле: `phone`, `code` или `password`.

Внутренние endpoints синхронизации:

```text
POST   /telegram/sync
GET    /telegram/folders?telegram_user_id=...
GET    /telegram/chats?telegram_user_id=...
```

`POST /telegram/sync` принимает `{"telegram_user_id": ...}`. Личные чаты по умолчанию не сохраняются. Ответы с кэшированными чатами содержат название, тип, unread count, mute/archive flags и last message id.

Внутренние endpoints групп источников:

```text
GET    /groups?telegram_user_id=...
POST   /groups
PATCH  /groups/{id}
DELETE /groups/{id}
GET    /groups/{id}/chats?telegram_user_id=...
POST   /groups/{id}/chats
DELETE /groups/{id}/chats/{chatId}
```

Create/update группы используют `telegram_user_id`, `name` и опциональный `description`. Добавление чата использует `telegram_user_id`, `chat_id`, опциональный `priority` и опциональный `enabled`.

Внутренний endpoint сбора сообщений:

```text
POST   /collections/group/{id}
```

Тело запроса:

```json
{"telegram_user_id": 123, "mode": "new", "limit": 100}
```

Поддерживаемые режимы: `new`, `24h`, `3d`, `week` и `latest_n`. Collection jobs сохраняют найденные сообщения, но намеренно не сдвигают `read_positions`; позиция будет обновляться только после успешного summary в следующем этапе.

Фильтрация и дедупликация сейчас работают как чистые Go-сервисы поверх collected messages:

- объединяют text и caption;
- нормализуют пробелы и переносы строк;
- удаляют типовые Telegram footers;
- извлекают URL;
- удаляют пустые, emoji-only, слишком короткие, рекламные и похожие на вакансии сообщения по правилам;
- группируют дубли по content hash, общей ссылке и token Jaccard similarity.

Внутренний endpoint summary:

```text
POST   /summaries/from-collection/{id}
GET    /summaries?telegram_user_id=...&limit=...
GET    /summaries/{id}?telegram_user_id=...
GET    /summaries/{id}/topics?telegram_user_id=...
```

Тело запроса:

```json
{"telegram_user_id": 123, "format": "standard"}
```

Генерация summary использует collected messages, filter/deduplication pipeline, OpenAI-compatible chat completions provider, строгую JSON validation, retry handling и сохраняет `summary_jobs`, `summaries` и `summary_topics`. Просмотр summary проверяет владельца и отдаёт последние summary, полный markdown одной summary и упорядоченные карточки тем для навигации в боте.

Внутренние endpoints статей:

```text
POST   /articles/from-summary/{id}
POST   /articles/from-summary/{id}/topics/{position}
GET    /articles?telegram_user_id=...&limit=...
GET    /articles/{id}?telegram_user_id=...
PATCH  /articles/{id}
```

Тело запроса конвертации:

```json
{"telegram_user_id": 123, "type": "analysis", "title": "Optional title", "tags": ["telegram", "ai"]}
```

Конвертация статей использует тот же OpenAI-compatible provider с отдельным JSON prompt, сохраняет draft в `articles`, source links в `article_sources`, генерирует уникальный slug и поддерживает owner-checked обновление title/tags.

Внутренние endpoints Obsidian export:

```text
POST   /exports/articles/{id}
POST   /exports/summaries/{id}
```

Тело запроса:

```json
{"telegram_user_id": 123}
```

Markdown exports сохраняются в `EXPORT_DIR`, содержат YAML frontmatter, используют безопасные `.md` имена файлов, сохраняют Telegram source links для черновиков статей, считают SHA-256 content hash и переиспользуют существующие записи `obsidian_exports` для идентичного контента. Бот отправляет созданный Markdown как Telegram document.

Если задан `OBSIDIAN_API_KEY`, exports также пишутся напрямую в Obsidian через [Local REST API plugin](https://github.com/coddingtonbear/obsidian-local-rest-api). `OBSIDIAN_REST_URL` задаёт base URL API, а `OBSIDIAN_INSECURE_SKIP_VERIFY=true` нужен для self-signed HTTPS certificate плагина. Существующие заметки перед update сохраняются как `*.backup-YYYYMMDD-HHMMSS.md`.

Scheduled summaries хранятся в `summary_schedules` и выполняются через `cmd/summary-worker`. MVP поддерживает daily schedule strings в формате `HH:MM`, `daily@HH:MM` или `@daily`, IANA timezones, quiet-hour windows, summary format и optional export to Obsidian. Каждая попытка записывается в `schedule_runs`.

## Docker

```sh
make docker-up
```

Docker images для `api`, `bot` и `tdlib-worker` собирают TDLib из официального репозитория `tdlib/td` и компилируют Go binaries с `-tags tdlib`. При необходимости revision исходников TDLib можно зафиксировать через build arg `TDLIB_GIT_REF`.

API будет доступен по адресам:

```text
http://localhost:8080/health
http://localhost:8080/ready
```

## База данных

Миграции создают:

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

Интеграционные тесты используют `TEST_DATABASE_URL`. Если переменная не задана, PostgreSQL integration tests пропускаются.

```sh
export TEST_DATABASE_URL='postgres://postgres:postgres@localhost:5432/telegram_summary_test?sslmode=disable'
go test ./internal/storage/postgres
```

## Безопасность

- `.env` игнорируется git'ом.
- Sensitive Telegram и LLM поля пока используются только как конфигурация.
- Логи не должны содержать тексты сообщений и secrets.
- Docker application containers запускаются от non-root пользователя.

## Следующий PR

Плановая цепочка из исходного roadmap выполнена до `PR-015`. Следующий этап — `PR-016 Secure Internal API`: service-token middleware, security headers, request body limits и HTTP server timeouts.
