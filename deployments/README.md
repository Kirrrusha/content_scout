# Production Deployment

Production runs on one Cloud.ru VM with Docker Compose. Images are built by
GitHub Actions and pushed to Cloud.ru Artifact Registry.

## Required GitHub Secrets

- `CLOUDRU_REGISTRY` (`kirrrusha.cr.cloud.ru` for the current Cloud.ru registry)
- `CLOUDRU_REGISTRY_KEY_ID`
- `CLOUDRU_REGISTRY_KEY_SECRET`
- `CLOUDRU_SECRET_KEY_ID`
- `CLOUDRU_SECRET_KEY_SECRET`
- `SSH_HOST`
- `SSH_USER`
- `SSH_PRIVATE_KEY`
- `SSH_HOST_ED25519_FINGERPRINT`

Optional:

- `CLOUDRU_SECRET_ID`, if fetching by secret id is preferred over name.
- `CLOUDRU_SECRET_PROJECT_ID`, required only when `CLOUDRU_SECRET_ID` is not set.

## Cloud.ru Secret Manager

Create a JSON secret named `content-scout-prod-env` with these keys:

```json
{
  "APP_ENV": "production",
  "HTTP_ADDR": ":8080",
  "POSTGRES_PASSWORD": "change-me",
  "DATABASE_URL": "postgres://postgres:change-me@postgres:5432/telegram_summary?sslmode=disable",
  "SERVICE_TOKEN": "change-me",
  "INTERNAL_API_URL": "http://api:8080",
  "TELEGRAM_BOT_TOKEN": "change-me",
  "TELEGRAM_OWNER_ID": "123",
  "TELEGRAM_API_ID": "123",
  "TELEGRAM_API_HASH": "change-me",
  "TELEGRAM_PROXY_URL": "socks5://user:pass@host:1080",
  "TDLIB_DATABASE_DIR": "/data/tdlib",
  "LLM_PROVIDER": "openai",
  "LLM_BASE_URL": "",
  "LLM_API_KEY": "change-me",
  "LLM_MODEL": "change-me",
  "LLM_TIMEOUT": "3m",
  "EXPORT_DIR": "/data/exports"
}
```

`OBSIDIAN_*`, `LOG_*`, `SUMMARY_RETENTION`, `WORKER_ID`, and
`ENCRYPTION_KEY` can be added when needed.

## VM Bootstrap

From the VM, run:

```sh
/opt/content_scout/bootstrap-vm.sh
```

Or copy and run `deployments/scripts/bootstrap-vm.sh` manually as user `scout`.
It installs Docker, Docker Compose, `jq`, configures 2 GB swap, prepares
`/opt/content_scout`, and enables UFW with SSH open.

After the GitHub secrets and Cloud.ru secret are present, successful `ci` runs
on `main` trigger `.github/workflows/cd.yml`. It can also be started manually
with `workflow_dispatch`.

For the current VM `88.218.67.232`, set:

```text
SSH_HOST_ED25519_FINGERPRINT=SHA256:CEkOgBfy+DRljKbTvNZXiZ0AOq//vqg0SfSlntZ4JuM
```

The CD workflow verifies this fingerprint before trusting the host key. Registry
login is performed through SSH stdin with a temporary Docker config that is
removed after deploy.
