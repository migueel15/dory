import { type ReactNode } from "react";
import type { ModelChoice, NodeSettings, ThreadDefaults } from "../../api/client";
import { explicitModels, findModel, modelId } from "../../effort";
import { useProviders } from "../../components/launch-picker";
import { ErrorText, Select } from "../../components/ui";
import { ALL_NODES, NodeDiff, SettingRow, useNodeSettings, type SectionProps } from "./settings-node";

const NONE: ModelChoice = { provider: "", model: "", effort: "" };
const EMPTY: ThreadDefaults = { default_model: NONE, fallback_model: NONE, default_workspace: "" };
const WORKSPACES = [{ value: "checkout", label: "Rama actual (checkout)" }, { value: "worktree", label: "Nuevo worktree" }];

// ThreadsSettings is the "Hilos" section: what a new thread starts with on the
// selected node(s) and what happens to it when its work is merged.
export function ThreadsSettings({ node }: SectionProps) {
  const settings = useNodeSettings(node);
  const providers = useProviders(node === ALL_NODES ? "local" : node);
  const list = (providers.data ?? []).map((p) => ({ ...p, models: explicitModels(p.models) })).filter((p) => p.models.length > 0);
  const threads = { ...EMPTY, ...settings.get("threads") };
  const disabled = !settings.loaded || settings.pending;
  
  const modelPicker = (field: "default_model" | "fallback_model", title: string, help: ReactNode) => {
    const choice = threads[field];
    const provider = list.find((p) => p.id === choice.provider);
    const model = provider && findModel(provider.models, choice.model);
    const setChoice = (next: ModelChoice) => settings.setPath(["threads", field], next);
    return (
      <div className="space-y-1.5 text-sm">
        <div className="flex items-start gap-3">
          <span className="min-w-0 flex-1"><strong className="block font-medium">{title}</strong><span className="text-slate-600 dark:text-muted">{help}</span></span>
          <NodeDiff settings={settings} field={["threads", field]} format={(v) => { const c = v as ModelChoice | undefined; return c?.provider ? `${c.provider} · ${c.model}${c.effort ? ` · ${c.effort}` : ""}` : "sin definir"; }} />
        </div>
        <div className="grid gap-2 sm:grid-cols-3">
          <Select disabled={disabled} value={choice.provider} placeholder="Sin definir" onChange={(v) => {
            const first = list.find((p) => p.id === v)?.models[0];
            setChoice(first ? { provider: v, model: modelId(first), effort: "" } : NONE);
          }} options={[{ value: "", label: "Sin definir" }, ...list.map((p) => ({ value: p.id, label: p.name }))]} />
          <Select disabled={disabled || !provider} value={model ? modelId(model) : choice.model} onChange={(v) => setChoice({ ...choice, model: v, effort: "" })}
            options={(provider?.models ?? []).map((m) => ({ value: modelId(m), label: m.name }))} />
          <Select disabled={disabled || !model?.efforts?.length} value={choice.effort} placeholder="Razonamiento" onChange={(v) => setChoice({ ...choice, effort: v })}
            options={[{ value: "", label: "Por defecto" }, ...(model?.efforts ?? []).map((e) => ({ value: e, label: e }))]} />
        </div>
      </div>
    );
  };

  return (
    <section aria-labelledby="threads-title" className="space-y-4">
      <h2 id="threads-title" className="text-lg font-semibold">Hilos</h2>
      {modelPicker("default_model", "Modelo por defecto", "Lo que usan los hilos nuevos de este nodo. Sin definir, se usa el último modelo elegido.")}
      {modelPicker("fallback_model", "Modelo de respaldo", "Se usa si el proveedor seleccionado está desactivado o no está instalado en este nodo.")}
      <div className="flex items-start gap-3 text-sm">
        <span className="min-w-0 flex-1"><strong className="block font-medium">Workspace por defecto</strong><span className="text-slate-600 dark:text-muted">Dónde empiezan los hilos nuevos de un proyecto. Puedes cambiarlo al crear cada hilo.</span></span>
        <div className="w-56 shrink-0"><Select disabled={disabled} value={threads.default_workspace || "checkout"} onChange={(v) => settings.setPath(["threads", "default_workspace"], v)} options={WORKSPACES} /></div>
        <NodeDiff settings={settings} field={["threads", "default_workspace"]} format={(v) => (v as string | undefined) || "checkout"} />
      </div>
      <SettingRow settings={settings} field="archive_threads_on_finish" title="Archivar hilos al mergear su rama"
        help="Cuando la pull request de la rama de un hilo se mergea (o la tarea se cierra o cancela), se archivan los hilos de esa tarea y se detienen los que estén en marcha." />
      <ErrorText error={settings.error ?? providers.error} />
    </section>
  );
}
