#!/usr/bin/env bash
# Mantiene un único entorno de desarrollo de Dory en segundo plano (unidad systemd de usuario).
#   start   para el entorno anterior y lanza scripts/dev.sh desde este checkout o worktree
#   stop    lo para
#   status  muestra qué rama está levantada y dónde
# Mientras corre, vigila la PR de la rama: si se mergea (a cualquier rama), el entorno se cierra solo.
set -euo pipefail

UNIT=dory-dev
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
WEB_PORT="${DORY_DEV_WEB_PORT:-5173}"
API_PORT="${DORY_DEV_LISTEN:-127.0.0.1:8081}"
API_PORT="${API_PORT##*:}"

busy() { ss -Hltn "( sport = :$API_PORT or sport = :$WEB_PORT )" | grep -q .; }

# Cierra los entornos de desarrollo lanzados a mano con scripts/dev.sh (cualquier checkout):
# mata el grupo de procesos de quien escuche en los puertos de desarrollo si es vite o el binario de air.
stop_manual() {
	local pid cmd pgid
	for pid in $(ss -Hltnp "( sport = :$API_PORT or sport = :$WEB_PORT )" | grep -o 'pid=[0-9]*' | cut -d= -f2 | sort -u); do
		cmd="$(ps -o args= -p "$pid" 2>/dev/null || true)"
		[[ "$cmd" == *vite* || "$cmd" == */tmp/dory\ serve* ]] || continue
		pgid="$(ps -o pgid= -p "$pid" | tr -d ' ')"
		echo "Cerrando entorno de desarrollo manual (grupo $pgid)..."
		kill -TERM -- "-$pgid" 2>/dev/null || true
	done
}

case "${1:-}" in
start)
	systemctl --user stop "$UNIT" 2>/dev/null || true
	systemctl --user reset-failed "$UNIT" 2>/dev/null || true
	stop_manual
	for _ in $(seq 20); do
		busy || break
		sleep 0.5
	done
	if busy; then
		echo "Los puertos $API_PORT o $WEB_PORT siguen ocupados por algo que no es un entorno de Dory:" >&2
		ss -Hltnp "( sport = :$API_PORT or sport = :$WEB_PORT )" >&2
		exit 1
	fi
	branch="$(git -C "$ROOT" branch --show-current)"
	systemd-run --user --unit="$UNIT" --collect \
		--description="Dory dev: ${branch:-detached} ($ROOT)" \
		--working-directory="$ROOT" \
		--setenv=PATH="$PATH:$(go env GOPATH)/bin" \
		--setenv=HOME="$HOME" \
		--setenv=DORY_DEV_MERGE_POLL="${DORY_DEV_MERGE_POLL:-60}" \
		"$ROOT/scripts/dev-preview.sh" run "$branch" >/dev/null
	echo "Levantando ${branch:-HEAD} desde $ROOT..."
	for _ in $(seq 120); do
		if ! systemctl --user is-active -q "$UNIT"; then
			echo "El entorno ha fallado:" >&2
			journalctl --user -u "$UNIT" -n 30 --no-pager >&2
			exit 1
		fi
		if ss -Hltn "sport = :$API_PORT" | grep -q . && curl -fs -o /dev/null "http://127.0.0.1:$WEB_PORT/"; then
			echo "Listo: http://$(hostname):$WEB_PORT"
			exit 0
		fi
		sleep 1
	done
	echo "Sigue arrancando; revisa: journalctl --user -u $UNIT -f" >&2
	;;
stop)
	systemctl --user stop "$UNIT" 2>/dev/null || true
	stop_manual
	echo "Entorno de desarrollo parado."
	;;
status)
	if systemctl --user is-active -q "$UNIT"; then
		systemctl --user show "$UNIT" -p Description --value
		echo "http://$(hostname):$WEB_PORT"
	else
		echo "No hay entorno de desarrollo levantado."
	fi
	;;
run)
	# Lo ejecuta la unidad: dev.sh más la vigilancia de la PR de la rama.
	branch="${2:-}"
	setsid "$ROOT/scripts/dev.sh" &
	dev=$!
	trap 'kill -TERM -- "-$dev" 2>/dev/null; wait "$dev" || true' EXIT INT TERM
	while kill -0 "$dev" 2>/dev/null; do
		sleep "${DORY_DEV_MERGE_POLL:-60}" &
		wait $! || true
		[[ -n "$branch" ]] || continue
		merged="$(cd "$ROOT" && gh pr list --head "$branch" --state merged --json number --jq 'length' 2>/dev/null || echo 0)"
		if [[ "$merged" != "0" ]]; then
			echo "La PR de $branch está mergeada: cierro el entorno de desarrollo."
			exit 0
		fi
	done
	wait "$dev"
	;;
*)
	echo "Uso: $0 start|stop|status" >&2
	exit 2
	;;
esac
