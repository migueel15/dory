import { useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Link } from "@tanstack/react-router";
import { ArrowBendLeftUp, CaretRight } from "@phosphor-icons/react";
import { api, type Thread } from "../api/client";
import { ProviderIcon } from "./launch-picker";

// useSubagents lists the children of a thread, archived ones included: they
// are the finished subagents.
export function useSubagents(id: string, enabled = true) {
  return useQuery({ queryKey: ["threads", "children", id], queryFn: () => api.threads({ parent_id: id }, true), enabled, refetchInterval: enabled ? 3000 : false });
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
  const stop = useMutation({ mutationFn: (id: string) => api.interrupt(id), onSettled: () => qc.invalidateQueries({ queryKey: ["threads"] }) });
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

// ParentLink shows in a subagent's header the thread that launched it.
export function ParentLink({ parentId }: { parentId: string }) {
  const parent = useQuery({ queryKey: ["thread-title", parentId], queryFn: () => api.threads({ id: parentId }, true).then((l) => l[0] ?? null), staleTime: 60_000 });
  return (
    <Link to="/threads/$threadId" params={{ threadId: parentId }} title="Hilo que lanzó este subagente"
      className="flex min-w-0 max-w-[40%] shrink items-center gap-1 truncate rounded px-1.5 py-0.5 text-xs text-slate-500 hover:bg-slate-100 dark:text-muted dark:hover:bg-raised">
      <ArrowBendLeftUp className="size-3.5 shrink-0" /><span className="truncate">{parent.data?.title || "Hilo padre"}</span>
    </Link>
  );
}
