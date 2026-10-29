# Instalación

dory corre como un servicio de systemd con el usuario `miguel`. Postgres corre en Docker Compose, con el puerto ligado a `127.0.0.1`. El acceso desde otras máquinas va por Tailscale.

## Requisitos

- Go, Docker con el plugin `compose` y `sudo`.
- `npm`, cuando exista `web/package.json`, para compilar la web que se embebe en el binario.
- `pg_dump` no hace falta en el host: el backup lo ejecuta dentro del contenedor.

## Instalar

```sh
scripts/install.sh
```

El script se puede repetir para actualizar. Hace lo siguiente:

1. Compila la web (si existe) y el binario Go.
2. Copia `dory` y `dory-backup` a `~/.local/bin`.
3. Crea `~/.config/dory/config.toml` y `~/.config/dory/dory.env` si no existen. El segundo lleva una contraseña aleatoria y permisos 600. Nunca sobrescribe ficheros existentes.
4. Levanta Postgres con `docker compose` y aplica las migraciones (`dory migrate up`).
5. Instala y arranca `dory.service` y `dory-backup.timer`.

Con `--no-systemd` hace todo menos el paso 5, útil para probar sin tocar `/etc/systemd`.

## Primer usuario y token

```sh
set -a; . ~/.config/dory/dory.env; set +a
dory user create
dory token create
```

El token se muestra una sola vez. Guárdalo para configurar el MCP (ver [mcp.md](mcp.md)).

## Operación

```sh
systemctl status dory
journalctl -u dory -f
systemctl list-timers dory-backup.timer
```

### Backups

`dory-backup.timer` ejecuta cada día a las 03:30 un `pg_dump` comprimido en `~/.local/share/dory/backups/dory-AAAAMMDD-HHMMSS.sql.gz`. Se conservan 14 días (`DORY_BACKUP_KEEP_DAYS`). Para lanzarlo a mano:

```sh
dory-backup
```

Para restaurar:

```sh
gunzip -c ~/.local/share/dory/backups/<fichero>.sql.gz | \
  docker compose -f ~/.local/share/dory/docker-compose.yml --env-file ~/.config/dory/dory.env \
  exec -T postgres psql -U dory -d dory
```

### Acceso por Tailscale

`dory` escucha en `127.0.0.1:8080`. Para exponerlo solo dentro de la tailnet:

```sh
sudo tailscale serve --bg 8080
```

Tailscale sirve HTTPS, así que `cookie_secure = true` funciona sin cambios.
