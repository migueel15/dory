# Configuración

dory lee, por este orden: valores por defecto, el fichero TOML y las variables `DORY_*`. El TOML es `~/.config/dory/config.toml`, o la ruta de `DORY_CONFIG`. Si `DORY_CONFIG` apunta a un fichero que no existe, es un error. El fichero por defecto es opcional.

Plantillas: [`config.example.toml`](../config.example.toml) y [`deploy/dory.env.example`](../deploy/dory.env.example).

| Clave TOML | Variable | Defecto | Descripción |
|---|---|---|---|
| `listen` | `DORY_LISTEN` | `127.0.0.1:8080` | Dirección `host:puerto` de escucha |
| `database_url` | `DORY_DATABASE_URL` | (obligatoria) | Cadena de conexión a Postgres |
| `cookie_secure` | `DORY_COOKIE_SECURE` | `true` | Cookie de sesión solo por HTTPS (`true` o `false`) |
| `session_ttl` | `DORY_SESSION_TTL` | `168h` | Duración de la sesión web |

## Secretos

Los secretos van en `~/.config/dory/dory.env`, con permisos 600. Lo leen la unidad systemd (`EnvironmentFile`), el backup y `docker compose --env-file`:

- `POSTGRES_PASSWORD`: contraseña de Postgres para `docker-compose.yml`.
- `POSTGRES_PORT`: puerto local de Postgres (opcional, 5432 por defecto).
- `DORY_DATABASE_URL`: debe usar la misma contraseña.

No guardes la contraseña en `config.toml`, que es legible por otros usuarios.
