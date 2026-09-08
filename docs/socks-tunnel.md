# SOCKS tunnel to the proxy VM

## Why it exists

TDLib always dials Telegram DCs by IP literal. DPI on the app VM egress path drops
plaintext SOCKS5 `CONNECT` packets whose payload carries a Telegram DC IP, while the
TCP connection itself stays open. The proxy then logs
`negotiate timeout after 30/31 seconds`, TDLib never obtains an auth key, and
`POST /telegram/auth/phone` fails with HTTP 400 after ~30s.

Verified on 2026-09-08 from the app VM, through one authenticated SOCKS connection:

| `CONNECT` target | result |
| --- | --- |
| `149.154.175.50:443` | dropped |
| `149.154.175.50:5222` | dropped |
| `api.telegram.org:443` (domain form) | succeeds |
| `1.1.1.1:443` | succeeds |

Wrapping the SOCKS session in SSH hides the handshake from DPI. The Telegram Bot API
path was never affected, because the Go SOCKS dialer sends the domain form.

## Layout

- App VM key: `/home/scout/.ssh/proxy_tunnel` (ed25519, no passphrase).
- Proxy VM `/root/.ssh/authorized_keys` entry for that key is restricted to
  `restrict,port-forwarding,permitopen="127.0.0.1:1080"` — port forwarding only,
  no shell, no pty, and only to the local `danted` port.
- App VM unit: `deployments/systemd/content-scout-socks-tunnel.service`, installed as
  `/etc/systemd/system/content-scout-socks-tunnel.service`.
- Its settings: `deployments/systemd/socks-tunnel.env.example`, installed as
  `/etc/content_scout/socks-tunnel.env` with mode `600`.
- The tunnel binds `172.18.0.1:1080`, the gateway of the compose network. The subnet is
  pinned in `deployments/compose/docker-compose.prod.yml` so the address survives network
  recreation, and `ufw` allows `172.18.0.0/16 -> 172.18.0.1:1080` only.
- `TELEGRAM_PROXY_URL` in Cloud.ru Secret Management (`content-scout-prod-env`) points at
  `socks5://<user>:<pass>@172.18.0.1:1080`. The credentials are still `danted`'s own.

## Install

```bash
sudo install -d -m 755 /etc/content_scout
sudo install -m 600 socks-tunnel.env.example /etc/content_scout/socks-tunnel.env
sudo install -m 644 content-scout-socks-tunnel.service /etc/systemd/system/
sudo systemctl daemon-reload
sudo systemctl enable --now content-scout-socks-tunnel.service
```

## Checks

```bash
systemctl is-active content-scout-socks-tunnel.service
ss -ltn | grep 172.18.0.1:1080
```

## If the subnet ever changes

Changing the compose subnet or gateway means updating, together:
`docker-compose.prod.yml`, `BIND_ADDRESS` in `/etc/content_scout/socks-tunnel.env`,
the `ufw` rule, and `TELEGRAM_PROXY_URL` in Cloud.ru Secret Management.
