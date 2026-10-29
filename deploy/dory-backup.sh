#!/usr/bin/env bash
# Backup diario de Postgres con pg_dump. Conserva los últimos KEEP_DAYS días.
set -euo pipefail

CONFIG_DIR="${DORY_CONFIG_DIR:-$HOME/.config/dory}"
COMPOSE_FILE="${DORY_COMPOSE_FILE:-$HOME/.local/share/dory/docker-compose.yml}"
BACKUP_DIR="${DORY_BACKUP_DIR:-$HOME/.local/share/dory/backups}"
KEEP_DAYS="${DORY_BACKUP_KEEP_DAYS:-14}"

mkdir -p "$BACKUP_DIR"
umask 077
out="$BACKUP_DIR/dory-$(date +%Y%m%d-%H%M%S).sql.gz"
tmp="$out.partial"

docker compose -f "$COMPOSE_FILE" --env-file "$CONFIG_DIR/dory.env" \
  exec -T postgres pg_dump -U dory -d dory --clean --if-exists | gzip > "$tmp"
mv "$tmp" "$out"

find "$BACKUP_DIR" -name 'dory-*.sql.gz' -mtime "+$KEEP_DAYS" -delete
echo "backup creado: $out"
