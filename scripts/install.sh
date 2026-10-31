#!/usr/bin/env bash
# Instala dory en esta máquina. Es idempotente: se puede repetir para actualizar.
# Uso: scripts/install.sh [--no-systemd]
# Necesita: go, docker (con compose), y npm si existe web/package.json.
# Las unidades systemd se instalan con sudo; con --no-systemd solo se prepara el usuario.
set -euo pipefail

REPO="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
BIN_DIR="$HOME/.local/bin"
CONFIG_DIR="${XDG_CONFIG_HOME:-$HOME/.config}/dory"
DATA_DIR="$HOME/.local/share/dory"
UNIT_DIR=/etc/systemd/system
USE_SYSTEMD=1
[ "${1:-}" = "--no-systemd" ] && USE_SYSTEMD=0

need() { command -v "$1" >/dev/null || { echo "falta $1" >&2; exit 1; }; }
need go
need docker

cd "$REPO"

if [ -f web/package.json ]; then
  need npm
  (cd web && npm ci && npm run build)
fi
go build -o dory ./cmd/dory

mkdir -p "$BIN_DIR" "$CONFIG_DIR" "$DATA_DIR/backups"
install -m 755 dory "$BIN_DIR/dory"
install -m 755 deploy/dory-backup.sh "$BIN_DIR/dory-backup"
install -m 755 deploy/dory-deploy.sh "$BIN_DIR/dory-deploy"
install -m 644 docker-compose.yml "$DATA_DIR/docker-compose.yml"

# Los ficheros de configuración existentes no se sobrescriben.
[ -f "$CONFIG_DIR/config.toml" ] || install -m 644 config.example.toml "$CONFIG_DIR/config.toml"
if [ ! -f "$CONFIG_DIR/dory.env" ]; then
  password="$(head -c 24 /dev/urandom | base64 | tr -d '/+=\n')"
  sed "s/cambiame/$password/g" deploy/dory.env.example > "$CONFIG_DIR/dory.env"
  echo "creado $CONFIG_DIR/dory.env con una contraseña aleatoria"
fi
chmod 600 "$CONFIG_DIR/dory.env"

docker compose -f "$DATA_DIR/docker-compose.yml" --env-file "$CONFIG_DIR/dory.env" up -d --wait postgres
set -a; . "$CONFIG_DIR/dory.env"; set +a
"$BIN_DIR/dory" migrate up

if [ "$USE_SYSTEMD" = 1 ]; then
  need sudo
  for unit in dory.service dory-backup.service dory-backup.timer dory-deploy.service dory-deploy.timer; do
    sudo install -m 644 "deploy/$unit" "$UNIT_DIR/$unit"
  done
  sudo systemctl daemon-reload
  sudo systemctl enable --now dory.service dory-backup.timer dory-deploy.timer
  sudo systemctl restart dory.service
fi

echo "dory instalado. Siguiente paso: dory user create y dory token create (ver docs/instalacion.md)."
