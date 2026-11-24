import { ArrowRight } from "@phosphor-icons/react";
import { type TurnModel } from "../../api/client";
import { claudeModelName } from "../../components/model-label";
import { ModelLogo } from "../../components/model-logo";
import { t } from "../../i18n/es";

export type ModelChange = { from: TurnModel; to: TurnModel; handoff: boolean };
export const providerLabels: Record<string, string> = { claude: "Claude Code", codex: "Codex", opencode: "OpenCode", antigravity: "Antigravity" };

export function readModelChange(detail?: string): ModelChange | undefined {
  try {
    const change = JSON.parse(detail ?? "") as ModelChange | null;
    return change?.from && change.to ? change : undefined;
  } catch {
    return undefined;
  }
}

export function transitionLabel(model: TurnModel, fallback: string) {
  const provider = providerLabels[model.provider] ?? model.provider;
  const id = model.reported_model || model.model;
  const name = id && id !== "default" ? claudeModelName(id) ?? id.replace(/^gpt-/, "GPT-") : fallback;
  return { provider, name };
}

export function ModelChangeDivider({ change, pending = false }: { change?: ModelChange; pending?: boolean }) {
  if (!change) return null;
  const from = transitionLabel(change.from, "Modelo anterior");
  const to = transitionLabel(change.to, "Modelo configurado");
  const caption = pending
    ? (change.handoff ? "Traspaso de contexto pendiente" : "Cambio de modelo pendiente")
    : change.handoff ? "Contexto transferido" : "";
  const rule = pending ? "border-t border-dashed border-slate-300 dark:border-border-strong" : "border-t border-slate-200 dark:border-border";
  return (
    <div role="separator"
      aria-label={`Cambio de modelo de ${from.provider} ${from.name} a ${to.provider} ${to.name}${caption ? `. ${caption}` : ""}`}
      className="my-5 flex items-center gap-3 text-center text-xs text-slate-600 dark:text-muted">
      <span className={`min-w-3 flex-1 ${rule}`} />
      <div className="min-w-0">
        <div className="flex flex-wrap items-center justify-center gap-x-2 gap-y-1">
          <span title={from.provider} className="inline-flex items-center gap-1">
            <ModelLogo driver={change.from.provider} />
            {from.name}
          </span>
          <ArrowRight weight="thin" aria-hidden="true" className="size-3.5 shrink-0" />
          <span title={to.provider} className="inline-flex items-center gap-1 font-medium text-slate-800 dark:text-text">
            <ModelLogo driver={change.to.provider} />
            {to.name}
          </span>
        </div>
        {caption && <p className="mt-1 text-[0.6875rem]">{caption}</p>}
      </div>
      <span className={`min-w-3 flex-1 ${rule}`} />
    </div>
  );
}

