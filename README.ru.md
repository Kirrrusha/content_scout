# Telegram Summary Bot

[English version](README.md)

Персональный сервис для сводок из Telegram с экспортом Markdown-файлов в Obsidian.

Текущее состояние репозитория: `PR-005 Telegram folders and chats`. Уже есть структура проекта, конфигурация, HTTP health checks, подключение PostgreSQL, миграции, Docker Compose, доменные сущности, repository interfaces, PostgreSQL repositories, shell Telegram-бота, state machine авторизации TDLib и pipeline синхронизации папок/чатов.

## Архитектура

- `cmd/api`: внутренний HTTP API. Доступны `/health` и `/ready`.
- `cmd/migrate`: легкий runner SQL-миграций.
- `cmd/bot`: процесс Telegram-бота с owner-only навигацией.
- `cmd/tdlib-worker`: shell TDLib worker.
- `cmd/summary-worker`: placeholder фонового summary worker для следующих PR.
- `internal/config`: конфигурация через переменные окружения.
- `internal/domain`: доменные сущности и enum'ы.
- `internal/storage`: repository interfaces.
- `internal/storage/postgres`: PostgreSQL connection, миграции и реализации repositories.
- `internal/telegram/bot`: Telegram Bot API polling, owner guard, меню, callback routing, просмотр кэша папок/чатов и in-memory dialog state.
- `internal/telegram/tdlib`: TDLib client interface, state machine авторизации, сохранение сессии, sync service для папок/чатов и placeholder native adapter.
- `migrations`: обратимые SQL-миграции.

Native TDLib adapter пока намеренно не подключен. Авторизация и синхронизация папок/чатов реализованы за интерфейсами и покрыты тестами с fake clients, поэтому реальный adapter можно добавить без изменения Bot/API flow.

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
/settings
```

Данные авторизации проходят через TDLib state machine. Номера телефонов, коды подтверждения и 2FA-пароли не логируются и не сохраняются приложением.

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

## Docker

```sh
make docker-up
```

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
- `articles`
- `article_sources`
- `obsidian_exports`

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

`PR-006 — Source groups`: CRUD пользовательских групп источников, добавление/удаление синхронизированных чатов, импорт из Telegram-папок после появления folder membership в native adapter и навигация по группам в боте.
