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

#### Merges seguidos y despliegues simultáneos

`dory-deploy.service` es `Type=oneshot`, así que systemd no solapa ejecuciones del timer. Cada ejecución despliega la punta de `main`; si llegan varios merges seguidos, el ciclo siguiente ve que la punta difiere de `deployed-commit` y la despliega. Los commits intermedios pueden saltarse, pero el resultado final es siempre el último.

Para evitar también despliegues simultáneos lanzados a mano, el script toma un cerrojo con `flock` sobre `~/.local/share/dory/deploy.lock`. Si hay otro despliegue en curso, imprime «despliegue en curso» y sale con 0. El cerrojo depende del proceso, no del fichero: se libera al terminar aunque falle y un `deploy.lock` huérfano no bloquea nada.

Si el último commit falla, queda en `failed-commit` y no se reintenta (ver arriba); el commit desplegado sigue siendo el anterior.

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

## Desarrollo

```sh
scripts/dev.sh   # web en http://localhost:5173, API en 127.0.0.1:8081
```

Requiere `air` (`go install github.com/air-verse/air@latest`). El script:

- Usa la base `dory_dev` del mismo Postgres. En cada arranque la borra y la copia de nuevo de `dory`, así que tiene los mismos usuarios, datos e instancias de T3.
- Aplica las migraciones a `dory_dev`; la base real no se toca.
- Arranca el backend con air (recompila y reinicia al guardar `.go` o `.sql`, según `.air.toml`) y la web con Vite, que aplica los cambios de `web/src` al instante y reenvía `/api` al backend.
- La web muestra un distintivo «dev» junto a «Dory» (solo con Vite en desarrollo, nunca en el build).
- Desactiva la cookie segura y deja vacío `DORY_PUBLIC_ORIGIN`: el backend compara `Origin` con `Host`, que el proxy de Vite conserva, así que el login funciona con cualquier nombre de host.

Producción sigue sirviendo en `:8080` con la base `dory`; el modo desarrollo usa `127.0.0.1:8081` (solo local) y `0.0.0.0:5173` (accesible desde la red y Tailscale, p. ej. `http://infra-admin:5173`) y puede correr a la vez. Si un puerto está ocupado, el script se detiene antes de tocar nada.

Variables opcionales: `DORY_ENV_FILE`, `DORY_DEV_DB`, `DORY_DEV_LISTEN` y `DORY_DEV_WEB_PORT`.

La instancia de desarrollo habla con las instancias reales de T3: lo que se lance desde ella (hilos, títulos con IA) ocurre de verdad, y sus notificaciones push se suman a las de producción.

### Probar una rama

Para probar una rama sin mergearla, desde su checkout o worktree:

```sh
scripts/dev-preview.sh start    # cierra cualquier entorno de desarrollo abierto y levanta esta rama
scripts/dev-preview.sh status   # rama levantada y URL
scripts/dev-preview.sh stop     # lo para
```

Corre en segundo plano como unidad systemd de usuario `dory-dev` (logs: `journalctl --user -u dory-dev -f`) y queda en `http://infra-admin:5173` hasta que se levanta otra rama, se para o se mergea la PR de la rama (a cualquier rama destino): el servicio lo comprueba cada minuto con `gh` y se cierra solo. Al mergearse, Dory ya pasa el item a `done` por su cuenta. Solo hay un entorno de desarrollo a la vez: si dos ramas se lanzan a la vez, se esperan y gana la última; para volver a otra, se relanza `start` desde su checkout. Cada `start` vuelve a copiar producción en `dory_dev`. La skill del proyecto `dory-dev-preview` (`.agents/skills/`, enlazada en `.claude/skills/`) permite pedírselo a un agente (`/dory-dev-preview` en Claude Code, `$dory-dev-preview` en Codex). Solo se ejecuta a petición: los agentes no la lanzan por su cuenta.
