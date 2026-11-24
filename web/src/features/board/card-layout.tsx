import { type Card } from "../../api/client";
import { slot } from "./card-status";

// Cards take the height their content needs; the column measures them to know how many fit.
export const MORE_H = 32;
export const GAP = 8;
export const PAD = 8;

// ago is the short relative time of the last activity.
export function ago(iso?: string | null): string {
  const t = iso ? Date.parse(iso) : NaN;
  if (Number.isNaN(t)) return "";
  const m = Math.floor((Date.now() - t) / 60_000);
  if (m < 1) return "ahora";
  if (m < 60) return `${m} min`;
  const h = Math.floor(m / 60);
  if (h < 24) return `${h} h`;
  return `${Math.floor(h / 24)} d`;
}

export const lastActivity = (c: Card) => c.threads.reduce((a, t) => { const x = t.last_activity_at ?? t.updated_at; return x > a ? x : a; }, c.updated_at);

// status is the colored text at the foot of a card. busy marks work in progress on GitHub (checks running).
export function status(c: Card): { text: string; cls: string; busy?: boolean } {
  if (c.blockers.length > 0) return { text: "Bloqueada", cls: "text-red-600 dark:text-red-400" };
  if (c.threads.some((t) => t.status === "working")) return { text: "Trabajando", cls: "text-blue-600 dark:text-blue-400" };
  if (c.threads.some((t) => t.status === "waiting")) return { text: "Te necesita", cls: "text-orange-600 dark:text-orange-400" };
  if (c.threads.some((t) => t.status === "error")) return { text: "Hilo fallido", cls: "text-red-600 dark:text-red-400" };
  const p = c.pull;
  if (p && c.pr_state !== "merged" && c.pr_state !== "closed") {
    const passed = p.ci_passed ?? 0, failed = p.ci_failed ?? 0, running = p.ci_pending ?? 0;
    const total = passed + failed + running;
    if (failed > 0 || p.ci === "FAILURE" || p.ci === "ERROR") return { text: failed > 0 ? `CI: ${failed} ${failed === 1 ? "falla" : "fallan"}` : "CI fallido", cls: "text-red-600 dark:text-red-400" };
    if (p.mergeable === "CONFLICTING") return { text: "Conflictos", cls: "text-red-600 dark:text-red-400" };
    if (running > 0 || p.ci === "PENDING" || p.ci === "EXPECTED") return { text: total > 0 ? `CI ${passed}/${total}` : "CI en curso", cls: "text-amber-600 dark:text-amber-400", busy: true };
    const approved = p.approved_by?.length ?? 0, waiting = p.review_requests.length;
    if (p.review_decision === "CHANGES_REQUESTED" || (p.changes_by?.length ?? 0) > 0) return { text: "Cambios pedidos", cls: "text-red-600 dark:text-red-400" };
    if (p.draft) return { text: "Draft", cls: "text-slate-500 dark:text-muted" };
    if (waiting > 0) return { text: approved > 0 ? `${approved}/${approved + waiting} aprobadas` : "Review pendiente", cls: "text-amber-600 dark:text-amber-400" };
    if (c.column === "ready_to_merge") return { text: approved > 1 ? `${approved}/${approved} aprobadas` : "Lista para merge", cls: "text-emerald-600 dark:text-emerald-400" };
    return { text: slot(c).label, cls: "text-amber-600 dark:text-amber-400" };
  }
  if (c.pr_state) return { text: slot(c).label, cls: "text-slate-500 dark:text-muted" };
  if (c.has_plan) return { text: "Con plan", cls: "text-indigo-600 dark:text-indigo-400" };
  return { text: c.column === "ready" ? "Lista" : "Sin empezar", cls: "text-slate-500 dark:text-muted" };
}

