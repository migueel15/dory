import { useMutation, useQueryClient } from "@tanstack/react-query";
import { useNavigate } from "@tanstack/react-router";
import { api, type Card, type Stage } from "../api/client";
import { askConfirm } from "./confirm";
import { ContextMenu, type ContextAction } from "./context-menu";

export type QuickLaunch = "define" | "implement";

// TaskQuickMenu is the right-click menu of a kanban card: the useful actions
// for the card's column, without opening the detail.
export function TaskQuickMenu({ c, x, y, onClose, onOpen, onError }: { c: Card; x: number; y: number; onClose: () => void; onOpen: (launch?: QuickLaunch) => void; onError: (e: unknown) => void }) {
  const qc = useQueryClient();
  const navigate = useNavigate();
  const refresh = () => { qc.invalidateQueries({ queryKey: ["board"] }); qc.invalidateQueries({ queryKey: ["card", c.id] }); };
  const stage = useMutation({ mutationFn: (s: Stage) => api.tasks.update(c.id, { stage: s }), onSuccess: refresh, onError: onError });
  const archive = useMutation({ mutationFn: () => api.tasks.archive(c.id), onSuccess: refresh, onError: onError });
  const latest = c.threads.length ? c.threads.reduce((a, b) => ((b.last_activity_at ?? b.updated_at) >= (a.last_activity_at ?? a.updated_at) ? b : a)) : undefined;
  const pullUrl = c.pull?.url ?? (c.repo && c.pr_number && /^[\w.-]+\/[\w.-]+$/.test(c.repo) ? `https://github.com/${c.repo}/pull/${c.pr_number}` : undefined);
  const a: ContextAction[] = [{ label: "Abrir", run: () => onOpen() }];
  const lastThread = () => latest && a.push({ label: "Ir al último hilo", run: () => navigate({ to: "/threads/$threadId", params: { threadId: latest.id } }) });
  const viewPull = () => pullUrl && a.push({ label: "Ver PR en GitHub", run: () => window.open(pullUrl, "_blank", "noreferrer") });
  const markDone = () => !c.pr_number && a.push({ label: "Marcar como hecha", run: () => stage.mutate("done") });
  switch (c.column) {
    case "inbox":
      a.push({ label: "Definir plan", run: () => onOpen("define") },
        { label: "Implementar directamente", disabled: c.blockers.length > 0, run: () => onOpen("implement") },
        { label: "Marcar como lista", run: () => stage.mutate("ready") });
      markDone();
      a.push({ label: "Cancelar tarea", run: () => stage.mutate("cancelled") });
      break;
    case "ready":
      a.push({ label: c.has_plan ? "Revisar plan" : "Definir plan", run: () => onOpen("define") },
        { label: "Implementar", disabled: c.blockers.length > 0, run: () => onOpen("implement") },
        { label: "Volver a inbox", run: () => stage.mutate("inbox") });
      markDone();
      a.push({ label: "Cancelar tarea", run: () => stage.mutate("cancelled") });
      break;
    case "working": case "needs_you":
      lastThread();
      a.push({ label: "Abrir otro hilo", run: () => onOpen("implement") });
      markDone();
      break;
    case "in_review": case "ready_to_merge":
      viewPull(); lastThread();
      break;
    case "done":
      viewPull();
      if (c.stage === "done" && !c.pr_number) a.push({ label: "Reabrir", run: () => stage.mutate("ready") });
      break;
    case "cancelled":
      a.push({ label: "Reabrir", run: () => stage.mutate("ready") });
      break;
  }
  a.push({ label: "Copiar ID", separated: true, run: () => { navigator.clipboard.writeText(c.id).catch(onError); } },
    { label: "Archivar", danger: true, run: () => { askConfirm("¿Archivar la tarea?", "Archivar").then((ok) => ok && archive.mutate()); } });
  return <ContextMenu x={x} y={y} actions={a} onClose={onClose} label={`Acciones de ${c.title}`} />;
}
