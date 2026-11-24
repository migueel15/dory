import { useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Link } from "@tanstack/react-router";
import { ArrowBendLeftUp, CaretRight, Robot } from "@phosphor-icons/react";
import { api, type Message, type Thread } from "../api/client";
import { Md } from "./ui";
import { ProviderIcon } from "./launch-picker";
import { qk } from "../api/queries/keys";

// useSubagents lists the children of a thread, archived ones included: they
// are the finished subagents.
export function useSubagents(id: string, enabled = true) {
  return useQuery({ queryKey: qk.threadChildren(id), queryFn: () => api.threads({ parent_id: id }, true), enabled, refetchInterval: enabled ? 3000 : false });
}

type Group = "working" | "action" | "done";
const groupOf = (t: Thread): Group => t.archived_at ? "done" : t.status === "working" ? "working" : t.status === "waiting" || t.status === "error" ? "action" : "done";
const LABELS: Record<Group, string> = { working: "Trabajando", action: "Requieren acción", done: "Terminados" };
const DOT: Record<Group, string> = {
  working: "size-2.5 animate-spin rounded-full border-2 border-blue-500 border-t-transparent",
  action: "size-2 rounded-full bg-amber-500 motion-safe:animate-pulse",
  done: "size-2 rounded-full bg-emerald-500",
};

// Subagents is the section of the thread panel with the child threads,
// grouped by what they need: working, waiting for someone, or finished.
export function Subagents({ threadId, shown }: { threadId: string; shown: boolean }) {
  const children = useSubagents(threadId, shown);
  const [openDone, setOpenDone] = useState(false);
  const qc = useQueryClient();
  const stop = useMutation({ mutationFn: (id: string) => api.interrupt(id), onSettled: () => qc.invalidateQueries({ queryKey: qk.threads() }) });
  const list = children.data ?? [];
  if (!list.length) return null;
  const byGroup = (g: Group) => list.filter((t) => groupOf(t) === g).sort((a, b) => (b.last_activity_at ?? b.created_at).localeCompare(a.last_activity_at ?? a.created_at));
  const row = (t: Thread, g: Group) => (
    <div key={t.id} className="group flex items-center gap-2.5 rounded-lg px-1.5 py-1.5 hover:bg-slate-100 dark:hover:bg-raised">
      <span className="grid size-4 shrink-0 place-items-center"><span className={DOT[g]} /></span>
      <Link to="/threads/$threadId" params={{ threadId: t.id }} className="min-w-0 flex-1" title={t.title}>
        <div className="truncate text-sm font-semibold">{t.title || "Subagente"}</div>
        <div className="flex items-center gap-1.5 truncate text-xs text-slate-500"><ProviderIcon driver={t.provider} model={t.model} compact /><span className="truncate">{t.model || t.provider}</span>
          {g === "action" && <span className="shrink-0 text-amber-600 dark:text-amber-400">{t.status === "error" ? "· error" : "· te necesita"}</span>}</div>
      </Link>
      {g === "working" && <button type="button" aria-label={`Parar ${t.title}`} title="Parar" onClick={() => stop.mutate(t.id)} disabled={stop.isPending}
        className="grid size-6 shrink-0 place-items-center rounded [@media(hover:hover)]:opacity-0 [@media(hover:hover)]:group-hover:opacity-100 focus:opacity-100"><span className="size-2.5 rounded-[2px] bg-red-500" /></button>}
    </div>
  );
  const done = byGroup("done");
  return (
    <div className="px-2.5 py-2">
      <div className="px-1.5 pb-1 text-xs text-slate-500">Subagentes</div>
      {(["working", "action"] as const).map((g) => {
        const rows = byGroup(g);
        return rows.length > 0 && <div key={g}>
          <div className="px-1.5 pt-1 text-[0.6875rem] font-medium uppercase tracking-wide text-slate-400">{LABELS[g]} · {rows.length}</div>
          {rows.map((t) => row(t, g))}
        </div>;
      })}
      {done.length > 0 && <div>
        <button type="button" onClick={() => setOpenDone(!openDone)} aria-expanded={openDone}
          className="flex w-full items-center gap-1 px-1.5 pt-1 text-left text-[0.6875rem] font-medium uppercase tracking-wide text-slate-400 hover:text-slate-600">
          <CaretRight className={`size-3 transition-transform ${openDone ? "rotate-90" : ""}`} />{LABELS.done} · {done.length}
        </button>
        {openDone && done.map((t) => row(t, "done"))}
      </div>}
      {stop.error && <p className="px-1.5 text-xs text-red-600">{stop.error.message}</p>}
    </div>
  );
}

// Related lists the threads this thread's agent created with thread_create:
// ordinary threads of the list, linked here only by where they came from.
export function Related({ threadId, shown }: { threadId: string; shown: boolean }) {
  const related = useQuery({ queryKey: qk.threadRelated(threadId), queryFn: () => api.threads({ origin_id: threadId }), enabled: shown, refetchInterval: shown ? 5000 : false });
  const list = (related.data ?? []).filter((t) => !t.archived_at);
  if (!list.length) return null;
  return (
    <div className="px-2.5 py-2">
      <div className="px-1.5 pb-1 text-xs text-slate-500">Hilos creados desde aquí</div>
      {list.map((t) => (
        <Link key={t.id} to="/threads/$threadId" params={{ threadId: t.id }} title={t.title} className="flex items-center gap-2.5 rounded-lg px-1.5 py-1.5 hover:bg-slate-100 dark:hover:bg-raised">
          <ProviderIcon driver={t.provider} model={t.model} compact />
          <span className="min-w-0 flex-1 truncate text-sm">{t.title || "Hilo"}</span>
          {t.status === "working" && <span className="size-2.5 shrink-0 animate-spin rounded-full border-2 border-blue-500 border-t-transparent" />}
        </Link>
      ))}
    </div>
  );
}

// ParentLink shows in a thread's header the thread it came from: the parent
// that launched a subagent, or the thread whose agent created it.
export function ParentLink({ parentId, origin = false }: { parentId: string; origin?: boolean }) {
  const parent = useQuery({ queryKey: qk.threadTitle(parentId), queryFn: () => api.threads({ id: parentId }, true).then((l) => l[0] ?? null), staleTime: 60_000 });
  return (
    <Link to="/threads/$threadId" params={{ threadId: parentId }} title={origin ? "Hilo desde el que se creó" : "Hilo que lanzó este subagente"}
      className="flex min-w-0 max-w-[40%] shrink items-center gap-1 truncate rounded px-1.5 py-0.5 text-xs text-slate-500 hover:bg-slate-100 dark:text-muted dark:hover:bg-raised">
      <ArrowBendLeftUp className="size-3.5 shrink-0" /><span className="truncate">{origin && "Creado desde "}{parent.data?.title || "Hilo padre"}</span>
    </Link>
  );
}

// useThreadTitle reads the title of any thread, archived ones included.
function useThreadTitle(id: string) {
  return useQuery({ queryKey: qk.threadTitle(id), queryFn: () => api.threads({ id }, true).then((l) => l[0] ?? null), staleTime: 60_000 }).data?.title;
}

// The header Dory writes on messages between agents: a subagent handing its
// result to its parent, or thread_send. The harness history may give the
// message back without its sender, so the header alone identifies it.
const AGENT_HEADER = /^\[(?:Subagente «(.*?)» \(([0-9a-f-]{36})\) ([^\]\n]+)|Mensaje del hilo «(.*?)» \(([0-9a-f-]{36})\))\]\s*/;
// Harness markers such as codex web citations (U+E200 … U+E201) are not text.
const MARKERS = /[^]*/g;

export type AgentNote = { sender: string; title: string; state: string; body: string };

// agentNote reads a user message written by another thread's agent, or null.
export function agentNote(m: Message): AgentNote | null {
  const match = m.text.match(AGENT_HEADER);
  if (!match && !m.sender_thread_id) return null;
  const body = (match ? m.text.slice(match[0].length) : m.text).replace(MARKERS, "").trim();
  if (match?.[2]) return { sender: m.sender_thread_id || match[2], title: match[1], state: match[3], body };
  if (match?.[5]) return { sender: m.sender_thread_id || match[5], title: match[4], state: "", body };
  return { sender: m.sender_thread_id!, title: "", state: "", body };
}

const TONE: Record<string, string> = { "terminó": "text-emerald-600 dark:text-emerald-400", "falló": "text-red-600 dark:text-red-400", "espera una respuesta": "text-amber-600 dark:text-amber-400", "se detuvo": "text-slate-500" };

// AgentMessage is a user turn another thread's agent wrote, such as a
// subagent handing over its result: a card folded to one line with its
// sender and state, so the user's own messages stay easy to find.
export function AgentMessage({ note }: { note: AgentNote }) {
  const [open, setOpen] = useState(false);
  const current = useThreadTitle(note.sender);
  const title = current || note.title || "Otro hilo";
  return (
    <div data-message className="w-full max-w-full">
      <div className="flex min-w-0 items-center gap-2 rounded-lg border border-slate-200 bg-slate-50 px-2.5 py-1.5 text-sm dark:border-border dark:bg-raised/40">
        <Robot className="size-4 shrink-0 text-slate-500" />
        <button type="button" onClick={() => note.body && setOpen(!open)} aria-expanded={open} disabled={!note.body} className="flex min-w-0 flex-1 items-center gap-1.5 text-left">
          {note.body && <CaretRight className={`size-3 shrink-0 text-slate-400 transition-transform ${open ? "rotate-90" : ""}`} />}
          <span className="shrink-0 text-slate-500 dark:text-muted">{note.state ? "Subagente" : "Mensaje de"}</span>
          <span className="min-w-0 truncate font-medium">{title}</span>
          {note.state && <span className={`shrink-0 ${TONE[note.state] ?? "text-slate-500"}`}>{note.state}</span>}
        </button>
        <Link to="/threads/$threadId" params={{ threadId: note.sender }} title="Abrir el hilo que lo envió" className="shrink-0 text-xs text-slate-500 hover:underline">Abrir</Link>
      </div>
      {open && note.body && <div className="mt-1 border-l-2 border-slate-200 pl-3 text-sm dark:border-border"><Md>{note.body}</Md></div>}
    </div>
  );
}
