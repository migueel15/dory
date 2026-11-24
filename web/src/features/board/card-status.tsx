import { CircleNotch, GitMerge, GitPullRequest } from "@phosphor-icons/react";
import { type Card } from "../../api/client";

// Fixed meaning of each state color (requirement RF-48).
export const TONE = {
  working: "text-blue-500", attention: "text-orange-500", ci: "text-yellow-500", review: "text-amber-500",
  ready: "text-emerald-500", merged: "text-purple-500", failed: "text-red-500", idle: "text-slate-400",
};

// slot is the single state icon of a card (requirement RF-47), with its meaning for the tooltip.
export function slot(c: Card): { icon: React.ReactNode; label: string } {
  if (c.threads.some((t) => t.status === "working")) return { icon: <CircleNotch weight="regular" className={`size-4 animate-spin ${TONE.working}`} />, label: "Un agente está trabajando" };
  const pull = c.pull;
  if (c.pr_state === "merged") return { icon: <GitMerge className={`size-4 ${TONE.merged}`} weight="fill" />, label: "PR fusionada" };
  if (c.pr_state === "closed") return { icon: <GitPullRequest className={`size-4 ${TONE.idle}`} />, label: "PR cerrada sin fusionar" };
  if (pull) {
    if (pull.ci === "FAILURE" || pull.ci === "ERROR") return { icon: <GitPullRequest className={`size-4 ${TONE.failed}`} weight="fill" />, label: "CI fallido" };
    if (pull.ci === "PENDING" || pull.ci === "EXPECTED") return { icon: <GitPullRequest className={`size-4 ${TONE.ci}`} weight="fill" />, label: "CI en ejecución" };
    if (pull.mergeable === "CONFLICTING") return { icon: <GitPullRequest className={`size-4 ${TONE.failed}`} weight="fill" />, label: "Conflictos de merge" };
    if (pull.review_decision === "CHANGES_REQUESTED" || pull.review_requests.length > 0) return { icon: <GitPullRequest className={`size-4 ${TONE.review}`} weight="fill" />, label: pull.review_decision === "CHANGES_REQUESTED" ? "Cambios pedidos" : "Review pendiente" };
    if (pull.draft) return { icon: <GitPullRequest className={`size-4 ${TONE.working}`} />, label: "PR en draft" };
    return { icon: <GitPullRequest className={`size-4 ${TONE.ready}`} weight="fill" />, label: "Lista para mergear" };
  }
  if (c.threads.some((t) => t.status === "waiting")) return { icon: <span className="size-2.5 rounded-full bg-orange-500" />, label: "Necesita tu respuesta" };
  if (c.threads.some((t) => t.status === "error")) return { icon: <span className="size-2.5 rounded-full bg-red-500" />, label: "El hilo falló" };
  return { icon: <span className="size-2.5 rounded-full bg-slate-300 dark:bg-slate-600" />, label: "Inactivo" };
}

