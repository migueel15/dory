import { useEffect, useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { Link } from "@tanstack/react-router";
import { ArrowUpLeft } from "@phosphor-icons/react";
import { api, type T3Thread, type T3ThreadView } from "../../api/client";
import { isWorking } from "../../api/thread-actions";
import { ModelLogo } from "../../components/model-logo";
import { statusLabel } from "../../components/thread-status";
import { detailsOpen, rememberDetails } from "./header";

// duration writes seconds as T3 Code does: "45s", "1m 59s", "1h 3m".
const duration = (secs: number) => {
  const s = Math.max(0, Math.round(secs));
  if (s < 60) return `${s}s`;
  if (s < 3600) return `${Math.floor(s / 60)}m ${s % 60}s`;
  return `${Math.floor(s / 3600)}h ${Math.floor((s % 3600) / 60)}m`;
};

const finished: Record<string, string> = {
  // A native subagent (Claude's Agent tool) ends idle, without runs of its own.
  completed: "Completado en", idle: "Completado en", interrupted: "Interrumpido tras", failed: "Falló tras", error: "Falló tras", cancelled: "Cancelado tras",
};

// SubagentBar takes the composer's place in a subagent's thread: its parent
// drives it, so it takes no messages. It shows the model, the effort and how
// its run went, and leads back to the parent.
export function SubagentBar({ v, t, parent }: { v: T3ThreadView; t: T3Thread; parent?: T3Thread }) {
  const catalog = useQuery({ queryKey: ["t3-catalog", v.instance_id], queryFn: () => api.t3Catalog(v.instance_id), enabled: !!v.instance_id, staleTime: 60_000 });
  const model = catalog.data?.providers.find((p) => p.instance_id === (t.provider || v.provider_id))?.models.find((m) => m.slug === (t.model || v.model))?.name ?? (t.model || v.model);
  const effort = v.effort ? v.effort.charAt(0).toUpperCase() + v.effort.slice(1) : "";
  const running = isWorking(t.status);
  // While it runs, the elapsed time ticks every second.
  const [now, setNow] = useState(() => Date.now());
  useEffect(() => {
    if (!running) return;
    const id = setInterval(() => setNow(Date.now()), 1000);
    return () => clearInterval(id);
  }, [running]);
  // Native subagents have no runs: T3 Code times them from creation to their
  // last change, and so does this bar.
  const start = Date.parse(t.run_started_at || t.created_at || "");
  const end = running ? now : Date.parse(t.run_completed_at || t.updated_at || "");
  const took = Number.isFinite(start) && Number.isFinite(end) ? duration((end - start) / 1000) : "";
  const state = running ? `Trabajando${took ? ` · ${took}` : ""}` : finished[t.status] && took ? `${finished[t.status]} ${took}` : statusLabel(t.status, t.attention);
  const parentId = t.parent_id;
  return (
    <div className="flex flex-wrap items-center gap-x-3 gap-y-1 rounded-2xl border border-slate-200 bg-white px-4 py-3 text-sm shadow-sm dark:border-border-strong dark:bg-surface">
      <span className="grid size-5 shrink-0 place-items-center"><ModelLogo driver={t.provider || v.provider_id} model={t.model || v.model} /></span>
      <span className="font-semibold">{model}</span>
      {effort && <span className="text-slate-500 dark:text-muted">{effort}</span>}
      <span className={running ? "text-blue-600 dark:text-blue-400" : "text-slate-500 dark:text-muted"}>{state}</span>
      <span className="ml-auto text-slate-500 dark:text-muted">Se ejecuta por su cuenta</span>
      {parentId && (
        <Link to="/threads/$threadId" params={{ threadId: parentId }} title={parent?.title ? `Abrir «${parent.title}»` : "Abrir el hilo padre"}
          onClick={() => { if (detailsOpen(v.id)) rememberDetails(parentId, true); }}
          className="inline-flex items-center gap-1.5 font-semibold hover:underline">
          <ArrowUpLeft className="size-4" />Abrir padre
        </Link>
      )}
    </div>
  );
}
