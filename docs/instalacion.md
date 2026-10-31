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
2. Copia `dory`, `dory-backup` y `dory-deploy` a `~/.local/bin`.
3. Crea `~/.config/dory/config.toml` y `~/.config/dory/dory.env` si no existen. El segundo lleva una contraseña aleatoria y permisos 600. Nunca sobrescribe ficheros existentes.
4. Levanta Postgres con `docker compose` y aplica las migraciones (`dory migrate up`).
5. Instala y arranca `dory.service`, `dory-backup.timer` y `dory-deploy.timer`.

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
systemctl list-timers dory-backup.timer dory-deploy.timer
```

### Despliegue automático

`dory-deploy.timer` comprueba cada minuto `origin/main`. Si hay un commit nuevo, `dory-deploy` hace lo siguiente en la copia limpia `~/.local/share/dory/src`, sin tocar los repositorios de trabajo:

1. Ejecuta `go test ./...`.
2. Lanza `dory-backup`.
3. Ejecuta `scripts/install.sh`, que aplica las migraciones y reinicia el servicio y el MCP.
4. Comprueba que `http://127.0.0.1:8080/` responde (`DORY_DEPLOY_HEALTH_URL`) y guarda el commit en `~/.local/share/dory/deployed-commit`.

Para ver qué ha pasado:

```sh
journalctl -u dory-deploy -n 50
cat ~/.local/share/dory/deployed-commit
```

Si un paso falla, el commit queda en `~/.local/share/dory/failed-commit` y no se reintenta hasta que llegue otro a `main`. Si falla antes de instalar, sigue funcionando la versión anterior. Si fallan las migraciones o el arranque, no hay rollback automático: arregla `main` o restaura el backup. Para reintentar el mismo commit:

```sh
rm ~/.local/share/dory/failed-commit
```

Para pausar los despliegues:

```sh
sudo systemctl stop dory-deploy.timer
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
