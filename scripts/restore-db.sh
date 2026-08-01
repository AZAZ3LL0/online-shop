#!/usr/bin/env bash
# Restore a dump produced by backup-db.sh. A backup nobody has restored is not
# a backup, so this is the script the go-live checklist exercises once.
#
# Usage: scripts/restore-db.sh /var/backups/qzq-shop/shop-20260801T030000Z.sql.gz

set -euo pipefail

cd "$(dirname "$0")/.."

dump="${1:-}"
if [ -z "$dump" ] || [ ! -f "$dump" ]; then
	echo "usage: scripts/restore-db.sh <dump.sql.gz>" >&2
	exit 1
fi

COMPOSE="docker compose --env-file .env -f docker/compose.yml"

read -r -p "This overwrites the live database from $dump. Type 'restore' to go on: " confirm
if [ "$confirm" != "restore" ]; then
	echo "aborted" >&2
	exit 1
fi

# The app is stopped first so no handler writes into a half-restored schema.
$COMPOSE stop app
gunzip -c "$dump" | $COMPOSE exec -T postgres psql --username app --dbname shop --single-transaction
$COMPOSE start app

echo "restore: done, now check /healthz on the public domain"
