# План деплоя content_scout

## Обзор

Три блока работ:

1. **CI** — расширить существующий `ci.yml` стандартным Go-набором проверок.
2. **CD** — сборка и push образов в cloud.ru Artifact Registry, деплой на VM в cloud.ru через docker compose.
3. **Telegram-прокси** — отдельный дешёвый VPS вне РФ с SOCKS5-прокси, через который бот и tdlib-worker ходят в Telegram.

Текущий production target: одна VM `88.218.67.232`, SSH alias `content-scout`,
пользователь `scout`, Ubuntu 22.04 LTS. Бэкапы PostgreSQL и TDLib-сессии
на первом этапе намеренно не настраиваются.

---

## 1. CI (GitHub Actions)

Что уже есть: `go vet`, `go test` с Postgres 17, `go build`, docker build api-образа.

Что добавить (стандарт для Go-проектов):

| Проверка | Инструмент | Зачем |
|---|---|---|
| Форматирование | `gofmt -l .` (fail если непусто) | Единый стиль без споров |
| Линтер | `golangci-lint` (action `golangci/golangci-lint-action`) | Включает staticcheck, errcheck, ineffassign, unused и др. — де-факто стандарт |
| Гонки | `go test -race ./...` | У вас воркеры и очередь — data race реален |
| Уязвимости | `govulncheck ./...` (официальный от Go team) | Проверка зависимостей по Go vuln DB |
| Docker-сборка всех образов | matrix по Dockerfile'ам | Сейчас собирается только api |

Рекомендуемая структура jobs:

```
ci.yml
├── lint        # gofmt + golangci-lint (быстрый, без БД)
├── test        # go test -race, с postgres service (как сейчас)
├── vuln        # govulncheck
└── build       # go build + docker build (matrix: api, bot, migrate, summary-worker, tdlib-worker)
```

Замечания:

- `golangci-lint` требует конфиг `.golangci.yml` — начать с дефолтного набора, не включать всё сразу.
- tdlib-worker собирается с CGO и тегом `tdlib` — в CI нужен шаг с установленным TDLib либо сборка только внутри Docker-образа (проще второе: проверять компиляцию через `docker build`).
- Кэш Go-модулей `actions/setup-go@v5` включает автоматически.

## 2. CD — cloud.ru Artifact Registry + деплой

### 2.1 Registry

- Создать реестр в cloud.ru Artifact Registry → адрес вида `<registry-name>.cr.cloud.ru`.
- Создать сервисный аккаунт с ролью на push, получить Key ID / Key Secret.
- Положить в GitHub Secrets: `CLOUDRU_REGISTRY`, `CLOUDRU_KEY_ID`, `CLOUDRU_KEY_SECRET`.

### 2.2 Workflow `cd.yml`

Триггер: push в `main` (или тег `v*` — когда захочется явных релизов).

```
jobs:
  push-images:
    - docker login <registry>.cr.cloud.ru -u <key-id> -p <key-secret>
    - build + push каждого образа с двумя тегами: git SHA и latest
  deploy:
    needs: push-images
    - ssh на VM (secrets: SSH_HOST, SSH_KEY)
    - docker compose pull
    - запуск migrate one-shot контейнером (до рестарта сервисов)
    - docker compose up -d
```

### 2.3 Хостинг основной части

Рекомендация: **одна VM в cloud.ru + docker compose**, а не managed-контейнеры.
Причины: compose уже есть в репо, опыта с k8s/Container Apps мало, сервисов немного
(api, bot, summary-worker, tdlib-worker, postgres). Позже можно мигрировать.

На VM:

- Docker + docker compose plugin.
- `deployments/compose/docker-compose.prod.yml` — production compose-файл: образы из registry (не build), `restart: unless-stopped`.
- Секреты: источник правды Cloud.ru Secret Management; GitHub Actions подтягивает JSON-секрет, временно копирует его на VM, deploy-скрипт рендерит `/opt/content_scout/.env` с `chmod 600` и удаляет исходный JSON.
- SSH host key: CD сверяет ED25519 fingerprint из `SSH_HOST_ED25519_FINGERPRINT` перед добавлением host key в `known_hosts`.
- Registry login: пароль registry передается на VM через SSH stdin; Docker credentials пишутся только во временный `DOCKER_CONFIG`, который удаляется после деплоя.
- Postgres: контейнер с Docker volume, без публикации порта наружу.
- tdlib-worker: volume под `TDLIB_DATABASE_DIR` (сессия TDLib должна переживать рестарты).
- API: порт `8080` публикуется только на `127.0.0.1`, в интернет не открывается.

## 3. Telegram-прокси (VPS вне РФ)

Проблема: из cloud.ru (РФ) недоступны `api.telegram.org` (Bot API) и MTProto-серверы (TDLib).

### Рекомендуемый вариант: SOCKS5-прокси на зарубежном VPS

Почему SOCKS5, а не альтернативы:

- **TDLib имеет встроенную поддержку SOCKS5** (`AddProxy` / `setTdlibParameters` + proxy) — ноль изменений в сетевом стеке.
- Bot API клиент (`go-telegram-bot-api`): `tgbotapi.NewBotAPIWithClient(...)` с `http.Client`, у которого transport через `golang.org/x/net/proxy` (SOCKS5). Либо просто `HTTPS_PROXY` env — `http.DefaultTransport` его уважает.
- WireGuard-туннель (вариант B) прозрачнее, но требует маршрутизации подсетей Telegram и админства на обоих концах — сложнее поддерживать.
- Перенос бота целиком на зарубежный VPS (вариант C) — тогда БД надо открывать наружу или тянуть второй туннель; больше движущихся частей.

Настройка VPS (Hetzner / любой EU, ~4–5 €/мес):

1. Поднять SOCKS5: `dante-server` или `3proxy`, с логином/паролем.
2. Firewall (ufw/nftables): порт прокси открыт **только для статического IP VM в cloud.ru**.
3. В приложении: env `TELEGRAM_PROXY_URL=socks5://user:pass@vps-ip:1080`,
   используется ботом и tdlib-worker'ом; пустое значение = без прокси (локальная разработка).

Если провайдер начнёт резать «голый» SOCKS5 — заменить на shadowsocks/wireguard на том же VPS, интерфейс для приложения (env-переменная) не меняется.

### Изменения в коде

- `internal/telegram`: чтение `TELEGRAM_PROXY_URL`, прокси в `http.Client` для Bot API.
- TDLib-адаптер: передача proxy-настроек при инициализации клиента.
- Оба — опциональны через env, тесты не трогаются.

## 4. Порядок работ

1. **PR: CI-расширение** — golangci-lint + конфиг, gofmt, `-race`, govulncheck, docker build matrix.
2. **PR: поддержка TELEGRAM_PROXY_URL** в боте и tdlib-worker (можно тестировать локально без прокси).
3. **Руками: инфраструктура** — registry + сервисный аккаунт в cloud.ru; VM cloud.ru; VPS с SOCKS5; секреты в Cloud.ru Secret Management и GitHub.
4. **PR: production compose** + `cd.yml` (push образов + ssh-деплой + migrate).
5. **Первый деплой руками**, проверка: бот отвечает, tdlib-сессия живёт, summary-worker обрабатывает очередь.
6. **Дальше** — деплой автоматом с каждого merge в main.

## Секреты — где что лежит

| Секрет | Место |
|---|---|
| Registry key id/secret, SSH-ключ деплоя, SSH host fingerprint, Cloud.ru Secret Manager access key | GitHub Secrets |
| Токен бота, DATABASE_URL, LLM-ключи, TELEGRAM_PROXY_URL, TDLib api_id/api_hash | Cloud.ru Secret Management |
| Runtime `.env` | Генерируется на VM при деплое, `chmod 600`, не является источником правды |
| TDLib-сессия | Docker volume `tdlib_data` |
