import { useMutation, useQueryClient } from "@tanstack/react-query";
import { useNavigate } from "@tanstack/react-router";
import { api, type T3InstanceThreads, type T3Thread, type T3ThreadAction } from "./client";
import type { ContextAction } from "../components/context-menu";

const now = () => new Date().toISOString();

// patch applies an action to a thread as T3 will, so the list changes at once.
function patch(t: T3Thread, a: T3ThreadAction): T3Thread | null {
  switch (a.action) {
    case "rename": return { ...t, title: a.title ?? t.title };
    case "pin": return { ...t, pinned_at: now() };
    case "unpin": return { ...t, pinned_at: undefined, pin_order_key: undefined };
    case "settle": return { ...t, settled_at: now(), attention: false };
    case "unsettle": return { ...t, settled_at: undefined };
    case "archive": return { ...t, archived_at: now(), attention: false };
    case "unarchive": return { ...t, archived_at: undefined };
    case "snooze": return { ...t, snoozed_until: a.until, attention: false };
    case "unsnooze": return { ...t, snoozed_until: undefined };
    case "read": return { ...t, last_visited_at: now() };
    case "unread": return { ...t, last_visited_at: undefined };
    case "delete": return null;
    default: return t;
  }
}

// useThreadAction runs an organization command on a thread: the list updates
// optimistically, a fork opens the new thread and deleting the open thread
// goes back to the list.
export function useThreadAction() {
  const qc = useQueryClient();
  const navigate = useNavigate();
  return useMutation({
    mutationFn: ({ id, ...body }: T3ThreadAction & { id: string }) => api.t3ThreadAction(id, body),
    onMutate: async ({ id, ...a }) => {
      await qc.cancelQueries({ queryKey: ["t3-threads"] });
      const before = qc.getQueryData<T3InstanceThreads[]>(["t3-threads"]);
      if (before) qc.setQueryData<T3InstanceThreads[]>(["t3-threads"], before.map((inst) => ({
        ...inst, threads: inst.threads.flatMap((t) => (t.id === id ? [patch(t, a)].filter((x): x is T3Thread => !!x) : [t])),
      })));
      if (a.action === "rename") qc.setQueryData<{ title: string }>(["t3-view", id], (v) => (v ? { ...v, title: a.title ?? v.title } : v));
      return { before };
    },
    onError: (e, _v, ctx) => {
      if (ctx?.before) qc.setQueryData(["t3-threads"], ctx.before);
      window.alert(`No se pudo completar la acción: ${e instanceof Error ? e.message : String(e)}`);
    },
    onSuccess: (res, { id, action }) => {
      if (action === "fork" && res?.thread_id) navigate({ to: "/threads/$threadId", params: { threadId: res.thread_id } });
      if (action === "delete" && location.pathname.endsWith(`/threads/${encodeURIComponent(id)}`)) navigate({ to: "/threads" });
    },
    onSettled: (_r, _e, { id, action }) => {
      if (action === "read") return; // nothing else shows it
      void qc.invalidateQueries({ queryKey: ["t3-threads"] });
      void qc.invalidateQueries({ queryKey: ["t3-status"] });
      if (action !== "delete") void qc.invalidateQueries({ queryKey: ["t3-view", id], exact: true });
    },
  });
}

const at9 = (days: number) => { const d = new Date(); d.setDate(d.getDate() + days); d.setHours(9, 0, 0, 0); return d; };
const nextMonday = () => { const d = new Date(); return at9(((8 - d.getDay()) % 7) || 7); };
const inHours = (h: number) => new Date(Date.now() + h * 3_600_000);

export const isWorking = (s = "") => ["running", "working", "starting", "preparing"].includes(s.toLowerCase());
export const isSnoozed = (t: Pick<T3Thread, "snoozed_until">) => !!t.snoozed_until && Date.parse(t.snoozed_until) > Date.now();
export const isUnread = (t: Pick<T3Thread, "last_visited_at" | "updated_at">) => !!t.last_visited_at && t.updated_at > t.last_visited_at;

// threadMenu lists what can be done with a thread, as T3 Code's sidebar menu.
export function threadMenu(t: T3Thread, run: (a: T3ThreadAction) => void, opts: { onRename: () => void }): ContextAction[] {
  const out: ContextAction[] = [{ label: "Renombrar", run: opts.onRename }];
  out.push(t.pinned_at ? { label: "Desfijar", run: () => run({ action: "unpin" }) } : { label: "Fijar arriba", run: () => run({ action: "pin" }) });
  // T3 only marks unread a thread with a finished turn.
  if (isUnread(t)) out.push({ label: "Marcar como leído", run: () => run({ action: "read" }) });
  else if (t.status && t.status !== "idle" && !isWorking(t.status)) out.push({ label: "Marcar como no leído", run: () => run({ action: "unread" }) });
  if (isSnoozed(t)) out.push({ label: "Despertar ahora", run: () => run({ action: "unsnooze" }) });
  else out.push({
    label: "Posponer", children: [
      { label: "1 hora", run: () => run({ action: "snooze", until: inHours(1).toISOString() }) },
      { label: "3 horas", run: () => run({ action: "snooze", until: inHours(3).toISOString() }) },
      { label: "Mañana a las 9:00", run: () => run({ action: "snooze", until: at9(1).toISOString() }) },
      { label: "El lunes a las 9:00", run: () => run({ action: "snooze", until: nextMonday().toISOString() }) },
    ],
  });
  out.push(t.settled_at ? { label: "Reactivar", run: () => run({ action: "unsettle" }) } : { label: "Asentar", run: () => run({ action: "settle" }) });
  if (t.status && t.status !== "idle") out.push({ label: "Bifurcar desde el último turno", run: () => run({ action: "fork", title: `${t.title} (bifurcación)` }) });
  if (isWorking(t.status)) out.push({ label: "Parar el hilo", run: () => run({ action: "stop" }) });
  out.push(t.archived_at ? { label: "Desarchivar", run: () => run({ action: "unarchive" }) } : { label: "Archivar", run: () => run({ action: "archive" }) });
  out.push({ label: "Eliminar…", run: () => { if (confirm(`¿Eliminar el hilo «${t.title}»? No se puede deshacer.`)) run({ action: "delete" }); } });
  return out;
}
