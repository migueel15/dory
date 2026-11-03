import { useMemo, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { api, type T3CatalogModel } from "../api/client";
import { Button, ErrorText, Label, Modal, Select } from "../components/ui";
import { saveEffort, savedEffort } from "../effort";
import { useT3 } from "./attention";

// ProviderIcon marks the company behind a model with its initial and colour.
export function ProviderIcon({ driver }: { driver: string }) {
  const d = driver.toLowerCase();
  const [text, color] = d.includes("claude") ? ["A", "bg-orange-600"] : d.includes("codex") || d.includes("openai") ? ["O", "bg-emerald-700"] : d.includes("gemini") ? ["G", "bg-blue-600"] : [driver.slice(0, 1).toUpperCase() || "?", "bg-slate-500"];
  return <span className={`grid size-5 shrink-0 place-items-center rounded text-[10px] font-bold text-white ${color}`}>{text}</span>;
}

// DefineIssueDialog starts one thread that turns the issue into Dory items.
// Runs on an explicit click only; the agent never implements.
export function DefineIssueDialog({ repo, number, onClose }: { repo: string; number: number; onClose: () => void }) {
  const qc = useQueryClient();
  const t3 = useT3();
  const envs = useQuery({ queryKey: ["environments"], queryFn: () => api.environments.list() });
  const instances = t3.data?.filter((s) => s.state === "available") ?? [];
  const envName = (id: string) => envs.data?.find((e) => e.id === id)?.name ?? "";
  // infra-admin is the default machine: the server that runs the agents.
  const fallback = instances.find((s) => /infra/i.test(envName(s.environment_id))) ?? instances[0];
  const [instance, setInstance] = useState("");
  const instanceId = instance || fallback?.instance_id || "";
  const catalog = useQuery({ queryKey: ["t3-catalog", instanceId], queryFn: () => api.t3Catalog(instanceId), enabled: !!instanceId });
  const repoName = repo.split("/")[1]?.toLowerCase() ?? "";
  const matched = catalog.data?.projects.find((p) => p.root.toLowerCase().replace(/\/+$/, "").split("/").pop() === repoName || p.title.toLowerCase() === repoName);
  const [project, setProject] = useState("");
  const projectId = project || matched?.id || "";
  const [filter, setFilter] = useState("");
  const [choice, setChoice] = useState<{ provider: string; model: T3CatalogModel } | null>(null);
  const [effort, setEffort] = useState("");
  const models = useMemo(() => {
    const f = filter.trim().toLowerCase();
    return (catalog.data?.providers ?? []).flatMap((p) => p.models.map((m) => ({ provider: p, model: m })))
      .filter(({ provider, model }) => !f || `${provider.name} ${model.name} ${model.slug}`.toLowerCase().includes(f));
  }, [catalog.data, filter]);
  const effortValue = effort || (choice?.model.effort ? savedEffort(choice.model.slug, choice.model.effort.values) || choice.model.effort.default : "");
  const launch = useMutation({
    mutationFn: () => api.issueDefine(repo, number, {
      instance_id: instanceId, project_id: projectId, provider: choice!.provider, model: choice!.model.slug,
      effort_option: choice!.model.effort?.id, effort: choice!.model.effort ? effortValue : undefined,
    }),
    onSuccess: () => {
      if (choice?.model.effort) saveEffort(choice.model.slug, effortValue);
      qc.invalidateQueries({ queryKey: ["issue-threads"] });
      onClose();
    },
  });
  return (
    <Modal title={`Definir tareas de #${number}`} onClose={onClose}>
      <Label text="Máquina">
        <Select value={instanceId} onChange={(v) => { setInstance(v); setProject(""); setChoice(null); setEffort(""); }} options={instances.map((s) => ({ value: s.instance_id, label: envName(s.environment_id) || s.base_url }))} />
      </Label>
      <Label text="Proyecto de T3">
        <Select value={projectId} onChange={setProject} placeholder="Elige el proyecto del repositorio" options={(catalog.data?.projects ?? []).map((p) => ({ value: p.id, label: p.title }))} />
      </Label>
      <Label text="Modelo">
        <input value={filter} onChange={(e) => setFilter(e.target.value)} placeholder="Filtrar modelos…" className="w-full rounded border border-slate-300 bg-transparent px-2 py-1 text-sm dark:border-slate-600" />
        <div className="mt-1 max-h-56 overflow-auto rounded border border-slate-200 dark:border-slate-700">
          {models.map(({ provider, model }) => {
            const active = choice?.provider === provider.instance_id && choice.model.slug === model.slug;
            return (
              <button key={`${provider.instance_id}|${model.slug}`} type="button" onClick={() => { setChoice({ provider: provider.instance_id, model }); setEffort(""); }}
                className={`flex w-full items-center gap-2 px-2 py-1.5 text-left text-sm hover:bg-slate-100 dark:hover:bg-slate-800 ${active ? "bg-slate-100 font-medium dark:bg-slate-800" : ""}`}>
                <ProviderIcon driver={provider.driver || provider.name} />
                <span className="flex-1 truncate">{model.name}</span>
                <span className="text-xs text-slate-400">{provider.name}</span>
              </button>
            );
          })}
          {catalog.isSuccess && models.length === 0 && <p className="px-2 py-1.5 text-sm text-slate-500">Ningún modelo coincide.</p>}
        </div>
      </Label>
      {choice?.model.effort && (
        <Label text="Esfuerzo">
          <Select value={effortValue} onChange={setEffort} options={choice.model.effort.values.map((v) => ({ value: v, label: v }))} />
        </Label>
      )}
      <p className="text-xs text-slate-500 dark:text-slate-400">El agente analiza la issue y el repositorio y crea los items en Dory; no implementa nada.</p>
      <ErrorText error={catalog.error ?? launch.error} />
      <Button disabled={!choice || !projectId || launch.isPending} onClick={() => launch.mutate()}>Lanzar</Button>
    </Modal>
  );
}
