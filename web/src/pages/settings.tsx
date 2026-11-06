import { useEffect, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { api, type TitleSettings } from "../api/client";
import { Button, Card, ErrorText, Label, Select } from "../components/ui";
import { useT3 } from "./attention";
import { ProviderIcon } from "./launch-form";

const empty: TitleSettings = { enabled: false, instance_id: "", provider: "", model: "", effort: "", project: "", timeout_seconds: 120 };

// TitlesSection edits where and with which model the titles of new ideas are generated.
function TitlesSection() {
  const qc = useQueryClient();
  const saved = useQuery({ queryKey: ["title-settings"], queryFn: api.titleSettings });
  const [form, setForm] = useState<TitleSettings>(empty);
  useEffect(() => { if (saved.data) setForm(saved.data); }, [saved.data]);
  const set = (patch: Partial<TitleSettings>) => setForm((f) => ({ ...f, ...patch }));

  const t3 = useT3();
  const envs = useQuery({ queryKey: ["environments"], queryFn: () => api.environments.list() });
  const envName = (id: string) => envs.data?.find((e) => e.id === id)?.name ?? "";
  const catalog = useQuery({ queryKey: ["t3-catalog", form.instance_id], queryFn: () => api.t3Catalog(form.instance_id), enabled: !!form.instance_id });
  const model = catalog.data?.providers.find((p) => p.instance_id === form.provider)?.models.find((m) => m.slug === form.model);

  const save = useMutation({
    mutationFn: () => api.saveTitleSettings(form),
    onSuccess: (data) => qc.setQueryData(["title-settings"], data),
  });
  const dirty = JSON.stringify(form) !== JSON.stringify(saved.data ?? empty);
  const ready = !form.enabled || (!!form.instance_id && !!form.model);

  return (
    <Card className="max-w-xl space-y-3">
      <div>
        <h2 className="font-semibold">Títulos con IA</h2>
        <p className="text-sm text-slate-500">Al crear una idea, un hilo de T3 genera el título con este modelo. Lo que escribes queda como descripción.</p>
      </div>
      <ErrorText error={saved.error} />
      <label className="flex items-center gap-2 text-sm">
        <input type="checkbox" checked={form.enabled} onChange={(e) => set({ enabled: e.target.checked })} />Generar el título de las ideas nuevas
      </label>
      <Label text="Conexión (máquina de T3)">
        <Select value={form.instance_id} placeholder="Elige una máquina" onChange={(v) => set({ instance_id: v, provider: "", model: "", effort: "", project: "" })} options={(t3.data ?? []).map((s) => ({
          value: s.instance_id,
          label: `${envName(s.environment_id) || s.base_url}${s.state === "available" ? "" : ` (${s.state})`}`,
        }))} />
      </Label>
      <ErrorText error={catalog.error} />
      <Label text="Modelo">
        <Select value={form.provider && form.model ? `${form.provider}|${form.model}` : ""} placeholder="Elige un modelo" disabled={!catalog.data}
          onChange={(v) => { const [provider, slug] = v.split("|"); set({ provider, model: slug, effort: "" }); }}
          options={(catalog.data?.providers ?? []).flatMap((p) => p.models.map((m) => ({ value: `${p.instance_id}|${m.slug}`, label: m.name, group: p.name, icon: <ProviderIcon driver={p.driver || p.name} /> })))} />
      </Label>
      {model?.effort && (
        <Label text="Esfuerzo">
          <Select value={form.effort} placeholder="El más bajo del modelo" onChange={(v) => set({ effort: v })} options={model.effort.values.map((v) => ({ value: v, label: v }))} />
        </Label>
      )}
      <Label text="Proyecto de T3 del hilo">
        <Select value={form.project} placeholder="El primero de la máquina" disabled={!catalog.data} onChange={(v) => set({ project: v })} options={(catalog.data?.projects ?? []).map((p) => ({ value: p.id, label: p.title }))} />
      </Label>
      <Label text="Tiempo máximo (segundos)">
        <input type="number" min={0} max={3600} value={form.timeout_seconds} onChange={(e) => set({ timeout_seconds: Number(e.target.value) || 0 })}
          className="w-32 rounded border border-slate-300 bg-transparent px-2 py-1 text-sm dark:border-border" />
      </Label>
      {!ready && <p className="text-sm text-amber-600">Para activarlo, elige una máquina y un modelo.</p>}
      <ErrorText error={save.error} />
      <div className="flex items-center gap-3">
        <Button disabled={!dirty || !ready || save.isPending} onClick={() => save.mutate()}>Guardar</Button>
        {save.isSuccess && !dirty && <span className="text-sm text-slate-500">Guardado</span>}
      </div>
    </Card>
  );
}

export function Settings() {
  return (
    <div className="space-y-4">
      <h1 className="text-xl font-bold">Ajustes</h1>
      <TitlesSection />
    </div>
  );
}
