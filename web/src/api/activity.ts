import { useSyncExternalStore } from "react";
import type { Message } from "./client";

// What each working thread is doing right now, as a short label taken from the
// messages the node streams. It lives outside React so any card can read it.
const labels = new Map<string, string>();
const listeners = new Set<() => void>();

function set(id: string, label: string | undefined) {
  if (label === undefined ? !labels.has(id) : labels.get(id) === label) return;
  if (label === undefined) labels.delete(id);
  else labels.set(id, label);
  listeners.forEach((l) => l());
}

const base = (p: string) => p.split("/").filter(Boolean).pop() ?? p;
const clip = (s: string, n = 28) => (s.length > n ? `${s.slice(0, n - 1)}…` : s);

// describe turns a streamed message into what the agent is doing.
export function describe(m: Message): string | undefined {
  switch (m.role) {
    case "reasoning": return "Razonando…";
    case "assistant": return "Escribiendo…";
    case "plan": return "Planificando…";
    case "question": return "Esperando respuesta…";
    case "tool": {
      const t = m.tool;
      if (!t) return "Usando una herramienta…";
      switch (t.kind) {
        case "edit": { const f = t.files?.[0]?.path ?? t.path; return f ? `Editando ${clip(base(f))}` : "Editando archivos…"; }
        case "read": return t.path ? `Leyendo ${clip(base(t.path))}` : "Leyendo archivos…";
        case "command": return t.command ? `Ejecutando ${clip(t.command.trim().split(/\s+/).slice(0, 2).join(" "))}` : "Ejecutando un comando…";
        case "search": return t.pattern || t.query ? `Buscando ${clip(t.pattern ?? t.query ?? "")}` : "Buscando en el código…";
        case "web": return "Consultando la web…";
        case "todo": return "Actualizando tareas…";
        case "plan": return "Planificando…";
        case "agent": return "Delegando en un agente…";
        case "mcp": return t.tool ? `Usando ${clip(t.tool)}` : "Usando una herramienta…";
        case "question": return "Esperando respuesta…";
        default: return "Trabajando…";
      }
    }
    default: return undefined;
  }
}

// Fed by the node event stream (see useLiveUpdates).
export function noteThreadUpdate(u: { thread_id: string; message?: Message; status?: string }) {
  if (u.status && u.status !== "working") { set(u.thread_id, undefined); return; }
  const label = u.message && describe(u.message);
  if (label) set(u.thread_id, label);
}

// setActivity is for simulated threads.
export const setActivity = set;

export function useThreadActivity(id: string | undefined): string | undefined {
  return useSyncExternalStore(
    (l) => { listeners.add(l); return () => { listeners.delete(l); }; },
    () => (id ? labels.get(id) : undefined),
  );
}
