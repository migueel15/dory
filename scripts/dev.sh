#!/usr/bin/env bash
# Levanta Dory en modo desarrollo: backend con air y web con Vite HMR.
# Usa la base dory_dev, que se copia de dory en cada arranque.
set -euo pipefail

cd "$(dirname "$0")/.."
ENV_FILE="${DORY_ENV_FILE:-$HOME/.config/dory/dory.env}"
DEV_DB="${DORY_DEV_DB:-dory_dev}"
API_ADDR="${DORY_DEV_LISTEN:-127.0.0.1:8081}"
WEB_PORT="${DORY_DEV_WEB_PORT:-5173}"

command -v air >/dev/null || { echo "Falta air: go install github.com/air-verse/air@latest" >&2; exit 1; }
[[ -f "$ENV_FILE" ]] || { echo "No existe $ENV_FILE" >&2; exit 1; }

set -a
# shellcheck disable=SC1090
source "$ENV_FILE"
set +a

compose() { docker compose -p dory --env-file "$ENV_FILE" "$@"; }
psql_c() { compose exec -T postgres psql -U dory -v ON_ERROR_STOP=1 "$@"; }

[[ -n "$(compose ps -q --status running postgres)" ]] || compose up -d --wait postgres >/dev/null

echo "Copiando dory en $DEV_DB..."
psql_c -d postgres -qc "DROP DATABASE IF EXISTS $DEV_DB WITH (FORCE)"
psql_c -d postgres -qc "CREATE DATABASE $DEV_DB"
compose exec -T postgres pg_dump -U dory -d dory --no-owner \
	| psql_c -q -d "$DEV_DB" >/dev/null

export DORY_DATABASE_URL="${DORY_DATABASE_URL%%\?*}"
export DORY_DATABASE_URL="${DORY_DATABASE_URL%/*}/$DEV_DB?sslmode=disable"
export DORY_LISTEN="$API_ADDR"
export DORY_COOKIE_SECURE=false
export DORY_PUBLIC_ORIGIN="http://localhost:$WEB_PORT"
export DORY_DEV_API="http://$API_ADDR"

go run ./cmd/dory migrate

[[ -d web/node_modules ]] || (cd web && npm install)

trap 'kill 0' EXIT INT TERM
air &
(cd web && npm run dev -- --port "$WEB_PORT" --strictPort) &
wait
