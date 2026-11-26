#!/usr/bin/env bash
# Dory de escritorio en modo desarrollo: levanta el core y Vite HMR sobre la
# rama actual y abre Electron apuntando a Vite (sin tocar la app instalada).
set -euo pipefail

cd "$(dirname "$0")/.."
export DORY_DATA_DIR="${DORY_DATA_DIR:-$HOME/.dory/dev}"
export DORY_LISTEN="${DORY_DEV_LISTEN:-127.0.0.1:8081}"
export DORY_MESH_PORT="${DORY_MESH_PORT:-0}"
WEB_PORT="${DORY_DEV_WEB_PORT:-5173}"
WEB_URL="http://localhost:$WEB_PORT"

[ -d web/node_modules ] || (cd web && npm ci --include=dev)
[ -f web/dist/index.html ] || (cd web && npm run build)
[ -d desktop/node_modules ] || (cd desktop && npm ci --include=dev)

PIDS=()
cleanup() {
  for pid in "${PIDS[@]}"; do
    kill "$pid" 2>/dev/null || true
  done
}
trap cleanup EXIT INT TERM

go run ./cmd/dory serve &
PIDS+=($!)

(cd web && DORY_DEV_API="http://$DORY_LISTEN" npx vite --port "$WEB_PORT") &
PIDS+=($!)

for _ in $(seq 1 60); do
  if curl -sf "http://$DORY_LISTEN/api/health" >/dev/null && curl -sf "$WEB_URL/" >/dev/null; then
    break
  fi
  sleep 0.5
done

cd desktop && DORY_DESKTOP_URL="$WEB_URL" npm start
