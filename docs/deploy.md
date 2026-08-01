# Production deployment

Covers S8.1 (TLS, domain, real provider keys, `setWebhook`) and S8.2 (backups,
log rotation, monitoring). The go-live rehearsal on a real payment is in
[golive.md](golive.md).

Everything below runs on one VPS: `app`, `postgres` and `caddy` from
`docker/compose.yml`. Nothing in this document changes application behaviour —
the contracts stay exactly as `tech.md` freezes them.

## 1. Prerequisites

- VPS, 2 CPU / 2 GB RAM minimum, ports 80 and 443 open, Docker Engine with the
  compose plugin.
- Domain with an `A` record pointing at the VPS, already propagated
  (`dig +short shop.example`).
- NOWPayments account with an API key and an IPN secret.
- Bot created in `@BotFather`: token and username.

## 2. Server layout

```bash
sudo install -d -o "$USER" -g "$USER" /srv/qzq-shop
git clone git@github.com:AZAZ3LL0/online-shop.git /srv/qzq-shop
cd /srv/qzq-shop
cp .env.example .env
chmod 600 .env
```

`/srv/qzq-shop` is the path the deploy workflow pulls into; it must match the
`DEPLOY_PATH` repository secret.

## 3. Fill in .env

`.env` is never committed. Generate the secrets on the server:

```bash
openssl rand -hex 32
```

| Variable | Production value |
|---|---|
| `APP_ENV` | `prod` — switches slog to JSON and turns on HSTS |
| `APP_BASE_URL` | `https://shop.example`, no trailing slash |
| `APP_SECRET` | fresh 32+ byte random value, never the example one |
| `DATABASE_URL` | overridden by compose, leave the example value |
| `SITE_ADDRESS` | `shop.example` — the bare domain, this is what Caddy gets a certificate for |
| `POSTGRES_PASSWORD` | fresh random value; compose refuses to start without it |
| `PAYMENTS_PROVIDER` | `nowpayments` |
| `NOWPAYMENTS_API_KEY`, `NOWPAYMENTS_IPN_SECRET` | from the NOWPayments dashboard |
| `TELEGRAM_PROVIDER` | `telegram` |
| `TELEGRAM_BOT_TOKEN`, `TELEGRAM_BOT_USERNAME` | from `@BotFather` |
| `TELEGRAM_WEBHOOK_SECRET` | fresh random value, sent as `X-Telegram-Bot-Api-Secret-Token` |
| `TELEGRAM_WEBHOOK_PATH_SECRET` | fresh random value, becomes part of the callback path |
| `ADMIN_TELEGRAM_IDS` | your numeric Telegram id, comma separated |
| `BACKUP_DIR`, `BACKUP_KEEP_DAYS` | `/var/backups/qzq-shop`, `14` |

`APP_BASE_URL` and `SITE_ADDRESS` must agree: the CSRF middleware compares the
request `Origin` against `APP_BASE_URL`, so a mismatch rejects every form post.

## 4. First start

```bash
make up
docker compose --env-file .env -f docker/compose.yml run --rm app migrate
docker compose --env-file .env -f docker/compose.yml run --rm app admin-password <login> <telegram-id>
curl -sS https://shop.example/healthz     # -> ok
```

Do **not** run `app seed` in production: it writes the demo collection and 30
days of fake traffic.

Caddy takes the certificate from Let's Encrypt on first boot and renews it on
its own; nothing else has to be scheduled for TLS. It only works if the `A`
record already resolves to this host and ports 80/443 reach it.

## 5. NOWPayments

The IPN callback is registered per invoice, not per account: the application
sends `ipn_callback_url` on every `POST /v1/invoice` (tech.md §5.4), so there is
nothing to configure in the dashboard beyond the IPN secret. Verify the secret
in the dashboard matches `NOWPAYMENTS_IPN_SECRET` — a mismatch makes every
callback fail the signature check and land in `payment_events` with
`signature_ok=false`.

## 6. Telegram webhook

```bash
docker compose --env-file .env -f docker/compose.yml run --rm app set-webhook
```

The subcommand builds `APP_BASE_URL + /webhooks/telegram/<path secret>` and
calls `setWebhook` with `TELEGRAM_WEBHOOK_SECRET`. It refuses to run against
plain HTTP, against localhost, and while `TELEGRAM_PROVIDER=fake`, so a dev
machine cannot steal the webhook from production.

Check the result — this is the only place the callback URL is printed, so run
it where nobody is watching your screen:

```bash
set -a; . ./.env; set +a
curl -sS "https://api.telegram.org/bot$TELEGRAM_BOT_TOKEN/getWebhookInfo"
```

`pending_update_count` should be `0` and `last_error_message` absent.

> The bot callback route itself (`POST /webhooks/telegram/{secret}`) is S4.2 and
> is not on `main` yet. Until that slice is merged, `setWebhook` points Telegram
> at a 404 — register the webhook only after the S4 branch lands.

## 7. Backups

`scripts/backup-db.sh` dumps the database through the compose service, gzips
it, verifies the archive, and deletes dumps older than `BACKUP_KEEP_DAYS`.

```bash
sudo install -m 644 scripts/qzq-shop.cron /etc/cron.d/qzq-shop
./scripts/backup-db.sh          # run once by hand to confirm it works
```

The dump contains customer names and shipping addresses, so `BACKUP_DIR` is
`0700` and each dump `0600`. Copy them off the host (any object storage with a
write-only key) if the VPS itself is a single point of failure.

Restore is `scripts/restore-db.sh <dump.sql.gz>`. It stops `app`, replays the
dump in one transaction and starts `app` again. Rehearse it once on a throwaway
host before you need it.

## 8. Logs and rotation

- Application logs go to stdout as JSON (`APP_ENV=prod`) and are captured by the
  Docker json-file driver, capped at 5 × 10 MB per service in
  `docker/compose.yml`.
- Caddy writes its access and diagnostic logs into the `caddylogs` volume and
  rolls them itself (10 MB, 10 files, 30 days). No `logrotate` unit is needed;
  adding one would fight Caddy for the same files.
- `make logs` follows the application log; `docker compose ... logs caddy`
  follows the proxy.

## 9. Monitoring

Point an external uptime monitor at `https://shop.example/healthz`, one minute
interval, expect HTTP 200 and the body `ok`. The probe pings Postgres, so it
turns red on a dead database and not only on a dead process.

Caddy polls the same URL every 10 seconds and pulls the app out of rotation when
it fails, which is why the probe must stay cheap and unauthenticated.

Worth alerting on as well:

- certificate expiry under 14 days (any TLS monitor);
- disk usage over 80% on the VPS — dumps and container logs are what fills it;
- `notifications` rows stuck in `failed` (the outbox gave up after 5 attempts).

## 10. Routine deploys

Merging to `main` runs the gate, then the deploy workflow: `git pull`, build,
`app migrate`, `up -d`, and a `/healthz` poll that fails the run if the release
never comes up. Required repository secrets: `DEPLOY_HOST`, `DEPLOY_USER`,
`DEPLOY_SSH_KEY`, `DEPLOY_PATH`.

Rollback is `git checkout <previous sha> && make up`. Migrations are forward
only (tech.md §10.3), so a rollback that has to undo a schema change needs a new
migration, not a revert.
