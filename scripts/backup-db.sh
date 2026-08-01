#!/usr/bin/env bash
# Dump the shop database, compress it and drop dumps older than the retention
# window. Runs from cron on the VPS, not from the application (TASKS.md S8.2).
#
# Usage: scripts/backup-db.sh          (run from the repository root)
# Reads BACKUP_DIR and BACKUP_KEEP_DAYS from .env, see .env.example.

set -euo pipefail

cd "$(dirname "$0")/.."

if [ ! -f .env ]; then
	echo "backup: .env not found in $(pwd)" >&2
	exit 1
fi
set -a
# shellcheck disable=SC1091
. ./.env
set +a

BACKUP_DIR="${BACKUP_DIR:-/var/backups/qzq-shop}"
BACKUP_KEEP_DAYS="${BACKUP_KEEP_DAYS:-14}"
COMPOSE="docker compose --env-file .env -f docker/compose.yml"

mkdir -p "$BACKUP_DIR"
# The dump carries customer names and addresses, so nobody but the owner reads it.
chmod 700 "$BACKUP_DIR"

stamp="$(date -u +%Y%m%dT%H%M%SZ)"
target="$BACKUP_DIR/shop-$stamp.sql.gz"
tmp="$target.part"

# -T keeps compose from allocating a tty, otherwise the stream gets mangled.
# The dump is written to a .part file first: a crash mid-write must not leave
# behind something that looks like a finished backup.
# --clean --if-exists lets the dump be replayed into a database that already
# has the schema, which is the only situation a restore ever happens in.
$COMPOSE exec -T postgres pg_dump --username app --dbname shop \
	--format plain --no-owner --clean --if-exists |
	gzip -9 >"$tmp"

# A dump that gzip cannot read back is not a backup.
gzip -t "$tmp"
mv "$tmp" "$target"
chmod 600 "$target"

find "$BACKUP_DIR" -maxdepth 1 -name 'shop-*.sql.gz' -type f -mtime "+$BACKUP_KEEP_DAYS" -delete
find "$BACKUP_DIR" -maxdepth 1 -name 'shop-*.sql.gz.part' -type f -mtime +1 -delete

echo "backup: $target ($(du -h "$target" | cut -f1))"
