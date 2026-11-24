// The backend answers every failure with a stable code, a technical message in
// English and parameters. The texts the user reads live here, by code.
export type ErrorParams = Record<string, unknown>;

type Text = string | ((p: ErrorParams) => string);

// A key is "code" or "code:reason"; the reason param refines a generic code.
const TEXTS: Record<string, Text> = {
  "thread_busy": "espera a que termine el turno antes de cambiar de proveedor",
  "node_offline": (p) => (p.thread ? "el nodo que ejecuta este hilo no está conectado" : "el nodo no está conectado"),
  "confirm_required": "apagar un móvil físico necesita confirmación",
  "forbidden:origin": "origen no permitido",
  "forbidden:tunnel_off": "el túnel del navegador está desactivado en este nodo",
  "forbidden:tunnel_dory_port": "el túnel no llega a los puertos de Dory",
  "forbidden:pair_proof": "el otro nodo no demostró conocer el código",
  "forbidden:pair_rejected": (p) => `el otro nodo rechazó el emparejamiento: ${p.detail ?? ""}`,
  "unavailable:no_browser": "este nodo no tiene navegador",
  "unavailable:no_skills": "este nodo no instala skills",
  "upstream:github_login": "No se pudo contactar con GitHub para iniciar sesión.",
  "upstream": (p) => String(p.detail ?? ""),
  "device_error": (p) => String(p.detail ?? ""),
  "no_adb": "adb no está instalado en este nodo",
  "invalid_code": "pega el código de autorización en una sola línea",
  "in_progress": "la idea ya se está guardando",
  "invalid_icon": "icono no válido",
  "repository_not_cloned": "el repositorio no está clonado en este nodo",
  "unknown_worktree": "el worktree no pertenece al repositorio del proyecto",
  "plan_not_accepted": "acepta el plan de la tarea antes de implementarla",
  "payload_too_large:audio": "la grabación supera los 12 MiB",
  "payload_too_large:attachment": (p) => `el fichero pasa de ${p.mb} MB`,
  "branch_switch_failed": (p) => `git switch ${p.branch}: ${p.detail ?? ""}`,
  "dirty_worktree": "el worktree tiene cambios sin commitear",
  "shared_process_group": "el servicio comparte grupo con Dory, el agente o una shell",
  "no_request": "la petición ya no está pendiente",
  "job_input_closed": "el intento ya no admite códigos",
  "model_missing": "descarga el modelo de dictado antes de usarlo",
  "no_chromium": "no hay Chromium en este nodo: instala chromium (o google-chrome) o define DORY_CHROMIUM con su ruta",
  "user_control": "el usuario ha tomado el control del navegador: puedes leer y capturar, pero no actuar hasta que lo devuelva",
  "update_refused:dev": "esta compilación es local y no sigue ningún canal",
  "update_refused:busy": "hay turnos en marcha en este nodo",
  "update_refused:running": "ya se está actualizando",
  "update_refused": (p) => String(p.detail ?? ""),
  "internal:usage_turns": "No se pudo leer el consumo del hilo",
  "internal:usage_breakdown": "No se pudo leer el desglose de uso",
  "invalid_account:folder_not_absolute": "la carpeta debe ser una ruta absoluta",
  "invalid_account:multiple_accounts_unsupported": (p) => `${p.driver} no admite varias cuentas`,
  "invalid_account:name_missing": "falta el nombre",
  "invalid_account:folder_missing": "falta la carpeta",
  "invalid_account:folder_in_use": "otra cuenta ya usa esa carpeta",
  "invalid_account:default_account_locked": "la cuenta predeterminada no se elimina",
  "invalid_argument:pid_invalid": "PID inválido",
  "invalid_argument:worktree_root_not_absolute": "la ubicación de los worktrees debe ser una ruta absoluta",
  "invalid_argument:idea_incomplete": "la idea necesita un id y texto",
  "invalid_argument:websocket_required": "se requiere WebSocket",
  "invalid_argument:model_required": "selecciona un modelo para el proveedor de destino",
  "invalid_argument:open_pull_request": (p) => `La tarea tiene una pull request sin mergear (#${p.number}); no se puede marcar como hecha`,
  "invalid_argument:system_skill_archive": "las skills del sistema no se pueden archivar; desactívala",
  "invalid_argument:system_skill_readonly": "las skills del sistema no se pueden editar; haz un fork",
  "invalid_argument:system_skill_name": (p) => `${p.name} es una skill del sistema; haz un fork con otro nombre`,
  "invalid_argument:fork_enabled": (p) => `desactiva antes el fork ${p.name}`,
  "invalid_argument:fork_not_system": "solo se puede hacer fork de una skill del sistema",
  "invalid_argument:skill_no_conflict": "la skill no tiene conflictos en este nodo",
  "invalid_argument:skill_not_on_node": "la skill no debe estar en este nodo; solo se puede retirar su copia",
  "invalid_argument:no_conflict_in": (p) => `no hay conflicto en ${p.harness}`,
  "invalid_argument:node_skill_no_description": "el SKILL.md del nodo no tiene descripción",
  "invalid_argument:node_skill_too_large": "la skill del nodo ocupa más de 1 MB",
};

// textFor is the Spanish text of a code, or the technical message when the web
// has none for it.
export function textFor(code: string | undefined, message: string, params: ErrorParams = {}): string {
  if (code) {
    const text = (typeof params.reason === "string" && TEXTS[`${code}:${params.reason}`]) || TEXTS[code];
    if (text) return typeof text === "function" ? text(params) : text;
  }
  return message;
}

// errorText is what the user reads for any failure.
export function errorText(err: unknown): string {
  if (!(err instanceof Error)) return "";
  const e = err as Error & { code?: string; detail?: string; params?: ErrorParams };
  return e.code ? textFor(e.code, e.detail ?? e.message, e.params) : e.message;
}
