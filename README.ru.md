# Telegram Summary Bot

[English version](README.md)

Персональный сервис для сводок из Telegram с экспортом Markdown-файлов в Obsidian.

Текущее состояние репозитория: `PR-017 Local Logging and 24-hour Retention`. Уже есть структура проекта, конфигурация, HTTP health checks, protected internal API через service token, подключение PostgreSQL, миграции, Docker Compose, локальные structured logs с 24-hour retention, доменные сущности, repository interfaces, PostgreSQL repositories, shell Telegram-бота, native TDLib JSON adapter, state machine авторизации TDLib, pipeline синхронизации папок/чатов, пользовательские группы источников, jobs сбора сообщений, фильтрация, группировка дублей, генерация summary через LLM, просмотр истории summary, конвертация summary/topics в черновики статей, Markdown export для Obsidian, scheduled summary runs и optional Obsidian Local REST API note writes.

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

Задайте `SERVICE_TOKEN` для внутреннего API. Все endpoints, кроме `GET /health` и `GET /ready`, требуют:

```http
Authorization: Bearer <token>
```

### Переменные Окружения

| Переменная | Обязательна | Default | Описание |
|---|---:|---|---|
| `APP_ENV` | нет | `development` | Метка окружения для сервисов и логов. |
| `HTTP_ADDR` | нет | `:8080` | Адрес, на котором слушает API. |
| `DATABASE_URL` | да | local PostgreSQL URL в коде, Docker URL в `.env.example` | Строка подключения к PostgreSQL. |
| `SERVICE_TOKEN` | да для `cmd/api` | empty | Bearer token для всех внутренних API endpoints, кроме `/health` и `/ready`. |
| `LOG_FORMAT` | нет | `json` | Формат логов: `json` для Docker/production-like запусков или `text` для локальной разработки. |
| `LOG_LEVEL` | нет | `info` | Минимальный уровень логов: `debug`, `info`, `warn` или `error`. |
| `LOG_DIR` | нет | `./data/logs` локально, `/data/logs` в Docker | Директория для локальных rotated log files. Пустое значение отключает file logging. |
| `LOG_RETENTION` | нет | `24h` | Удалять log files старше этого duration. |
| `LOG_ROTATION_INTERVAL` | нет | `1h` | Интервал ротации файлов и cleanup loop. |
| `TELEGRAM_BOT_TOKEN` | да для `cmd/bot` | empty | Telegram Bot API token. Если пустой, бот выходит в idle-режиме. |
| `TELEGRAM_OWNER_ID` | да для действий bot/API | `0` | Telegram user id, которому разрешено управлять ботом и API flow. |
| `TELEGRAM_API_ID` | да для native TDLib | `0` | Telegram API id из my.telegram.org. |
| `TELEGRAM_API_HASH` | да для native TDLib | empty | Telegram API hash из my.telegram.org. |
| `TDLIB_DATABASE_DIR` | нет | `./data/tdlib` локально, `/data/tdlib` в Docker | Директория TDLib session/database. Её нужно сохранять между перезапусками. |
| `TDLIB_GIT_REF` | нет | `master` | Branch/tag/commit исходников TDLib для Docker-сборок. |
| `TDLIB_INTEGRATION_SESSION_DIR` | нет | temp dir | Директория сессии для опциональных native TDLib integration tests. |
| `LLM_PROVIDER` | нет | `openai` | Метка LLM provider. Текущий adapter OpenAI-compatible. |
| `LLM_BASE_URL` | зависит от provider | empty | Base URL OpenAI-compatible chat completions API. |
| `LLM_API_KEY` | да для summary/article generation | empty | LLM API key. Не логировать. |
| `LLM_MODEL` | да для summary/article generation | empty | Имя модели для генерации summary и статей. |
| `ENCRYPTION_KEY` | reserved | empty | Зарезервировано для будущего encrypted secret storage. |
| `EXPORT_DIR` | нет | `./data/exports` локально, `/data/exports` в Docker | Директория для Markdown exports. |
| `OBSIDIAN_REST_URL` | нет | empty | Base URL Obsidian Local REST API. |
| `OBSIDIAN_API_KEY` | нет | empty | API key для Obsidian Local REST API. Если задан, включает REST export. |
| `OBSIDIAN_INSECURE_SKIP_VERIFY` | нет | `false` | Разрешить self-signed HTTPS certificate Obsidian plugin. |

## Команды

| Команда | Описание |
|---|---|
| `make build` | Собрать все Go command binaries в обычном локальном режиме без native TDLib. |
| `make build-tdlib` | Собрать `api`, `bot` и `tdlib-worker` с `-tags tdlib` и включённым CGO. Требует локальный `libtdjson`. |
| `make test` | Запустить весь стандартный Go test suite. |
| `make test-tdlib-nocgo` | Запустить тесты TDLib package с tag `tdlib`, но без CGO, проверяя fallback path. |
| `make test-tdlib-integration` | Запустить опциональные native TDLib integration tests. Требует `libtdjson`, Telegram API credentials и директорию сессии. |
| `make lint` | Запустить `go vet ./...`. |
| `make migrate-up` | Применить SQL migrations из `migrations`. |
| `make migrate-down` | Откатить SQL migrations из `migrations`. |
| `make run-api` | Запустить внутренний HTTP API локально. |
| `make run-bot` | Запустить Telegram-бота локально. |
| `make run-tdlib` | Запустить TDLib worker entrypoint локально. |
| `make run-worker` | Запустить summary worker локально. |
| `make docker-up` | Создать `.env`, если его нет, и запустить Docker Compose с rebuild. |
| `make docker-down` | Остановить и удалить контейнеры Docker Compose. |

Бот уходит в idle-режим, если не заданы `TELEGRAM_BOT_TOKEN` или `TELEGRAM_OWNER_ID`. Если оба значения заданы, бот запускает Telegram long polling и отвечает только настроенному владельцу.

Текущие команды бота:

| Команда | Описание |
|---|---|
| `/start` | Открыть главное меню бота. |
| `/connect` | Начать или продолжить авторизацию Telegram-аккаунта через TDLib. |
| `/phone <number>` | Передать номер телефона, который запросил TDLib. |
| `/code <code>` | Передать код подтверждения входа в Telegram. |
| `/password <2fa password>` | Передать пароль 2FA, если он нужен аккаунту. |
| `/session` | Показать текущую TDLib-сессию и состояние авторизации. |
| `/delete_session` | Выйти из аккаунта и удалить сохранённую TDLib-сессию. |
| `/folders` | Показать сохранённый кэш Telegram-папок. |
| `/chats` | Показать сохранённый кэш Telegram-чатов. |
| `/sync` | Синхронизировать папки и чаты из Telegram через TDLib. |
| `/groups` | Показать настроенные группы источников. |
| `/group_create <name>` | Создать группу источников. |
| `/group_rename <id> <name>` | Переименовать группу источников. |
| `/group_delete <id>` | Удалить группу источников. |
| `/group_chats <id>` | Показать чаты, привязанные к группе источников. |
| `/group_add_chat <group_id> <chat_id> [priority]` | Добавить Telegram-чат в группу источников. |
| `/group_remove_chat <group_id> <chat_id>` | Удалить чат из группы источников. |
| `/collect_group <group_id> [new\|24h\|3d\|week\|latest_n] [limit]` | Собрать сообщения из всех включённых чатов группы. |
| `/summarize_collection <collection_job_id> [short\|standard\|detailed]` | Сгенерировать summary из collection job. |
| `/summaries` | Показать последние summary. |
| `/summary <summary_id>` | Показать одно summary целиком. |
| `/summary_topics <summary_id>` | Показать темы, извлечённые из summary. |
| `/topic <summary_id> <position>` | Показать конкретную тему summary. |
| `/article_from_summary <summary_id> [analysis\|guide\|educational\|outline\|telegram_post]` | Превратить всё summary в черновик статьи. |
| `/article_from_topic <summary_id> <position> [analysis\|guide\|educational\|outline\|telegram_post]` | Превратить одну тему summary в черновик статьи. |
| `/articles` | Показать последние черновики статей. |
| `/article <article_id>` | Показать один черновик статьи. |
| `/article_title <article_id> <новое название>` | Обновить название статьи. |
| `/article_tags <article_id> tag1,tag2` | Заменить теги статьи. |
| `/export_article <article_id>` | Экспортировать статью в Markdown и опционально в Obsidian REST. |
| `/export_summary <summary_id>` | Экспортировать summary в Markdown и опционально в Obsidian REST. |
| `/settings` | Открыть настройки и управление аккаунтом/сессией. |

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

## Локальные Логи

Сервисы используют `log/slog` и пишут одновременно в stdout и hourly files внутри `LOG_DIR`.

```text
data/logs/
├── api-2026-07-13-18.log
├── api-current.log
├── bot-2026-07-13-18.log
├── summary-worker-2026-07-13-18.log
└── tdlib-worker-2026-07-13-18.log
```

Файлы старше `LOG_RETENTION` удаляются лёгким cleanup loop внутри каждого процесса. В логи не должны попадать authorization codes, 2FA passwords, API keys, service tokens и полный текст приватных сообщений.

Примеры:

```sh
tail -f data/logs/summary-worker-current.log
grep '"level":"ERROR"' data/logs/*.log
```

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
- Внутренние API endpoints требуют `SERVICE_TOKEN`, кроме `/health` и `/ready`.
- Логи не должны содержать тексты сообщений и secrets.
- Docker application containers запускаются от non-root пользователя.

## Следующий PR

Плановая цепочка из исходного roadmap выполнена до `PR-017`. Следующий этап — `PR-018 Reliable Job Queue`.
