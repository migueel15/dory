#!/usr/bin/env bash
# Despliega origin/main si ha cambiado. Lo lanza dory-deploy.timer cada minuto.
# Un commit fallido no se reintenta hasta que llegue otro (ver docs/instalacion.md).
set -euo pipefail

REPO_URL="${DORY_DEPLOY_REPO:-https://github.com/migueel15/dory.git}"
DATA_DIR="${DORY_DATA_DIR:-$HOME/.local/share/dory}"
HEALTH_URL="${DORY_DEPLOY_HEALTH_URL:-http://127.0.0.1:8080/}"
SRC_DIR="$DATA_DIR/src"
DEPLOYED_FILE="$DATA_DIR/deployed-commit"
FAILED_FILE="$DATA_DIR/failed-commit"

[ -d "$SRC_DIR/.git" ] || git clone --quiet "$REPO_URL" "$SRC_DIR"
cd "$SRC_DIR"

git fetch --quiet origin main
target="$(git rev-parse FETCH_HEAD)"

[ "$target" = "$(cat "$DEPLOYED_FILE" 2>/dev/null || true)" ] && exit 0
[ "$target" = "$(cat "$FAILED_FILE" 2>/dev/null || true)" ] && exit 0

echo "desplegando $target"
# Se marca como fallido hasta que el despliegue termine bien.
echo "$target" > "$FAILED_FILE"

git checkout --quiet --force --detach "$target"
go test ./...
dory-backup
scripts/install.sh

for _ in $(seq 30); do
  if curl -fsS -m 2 -o /dev/null "$HEALTH_URL"; then
    echo "$target" > "$DEPLOYED_FILE"
    rm -f "$FAILED_FILE"
    echo "desplegado $target"
    exit 0
  fi
  sleep 1
done

echo "dory no responde en $HEALTH_URL tras desplegar $target" >&2
exit 1
