#!/usr/bin/env bash
# Dory en modo desarrollo: el nodo con go run sobre una carpeta de datos
# propia y la web con Vite HMR. Con DORY_REPLAY_FILE se puede probar sin agentes.
set -euo pipefail

cd "$(dirname "$0")/.."
export DORY_DATA_DIR="${DORY_DATA_DIR:-$HOME/.dory/dev}"
export DORY_LISTEN="${DORY_DEV_LISTEN:-127.0.0.1:8081}"
export DORY_MESH_PORT="${DORY_MESH_PORT:-0}"
export DORY_REPLAY_FILE="${DORY_REPLAY_FILE:-$PWD/internal/runtime/testdata/claude.jsonl}"
WEB_PORT="${DORY_DEV_WEB_PORT:-5173}"

[ -d web/node_modules ] || (cd web && npm ci --include=dev)
[ -f web/dist/index.html ] || (cd web && npm run build)
go run ./cmd/dory serve &
trap 'kill %1 2>/dev/null' EXIT
cd web && DORY_DEV_API="http://$DORY_LISTEN" npx vite --port "$WEB_PORT"
