#!/usr/bin/env bash
# Compila e instala Dory en esta máquina y deja el núcleo como servicio de usuario.
# Es idempotente: se puede repetir para actualizar desde el código.
# Uso: scripts/install.sh [--no-service]
set -euo pipefail

REPO="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
BIN_DIR="$HOME/.local/bin"

need() { command -v "$1" >/dev/null || { echo "falta $1" >&2; exit 1; }; }
need go
need npm

cd "$REPO"
(cd web && npm ci --include=dev && npm run build)
go build -o dory ./cmd/dory
mkdir -p "$BIN_DIR"
install -m 755 dory "$BIN_DIR/dory"
rm -f dory

if [ "${1:-}" != "--no-service" ]; then
  "$BIN_DIR/dory" service install
  "$BIN_DIR/dory" restart || true
fi
"$BIN_DIR/dory" status
echo "Abre $("$BIN_DIR/dory" url) o ejecuta dory open."
