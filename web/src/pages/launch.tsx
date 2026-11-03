import { useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { api, type Item } from "../api/client";
import { Button, ErrorText, Label, Modal, Select } from "../components/ui";
import { saveEffort, savedEffort } from "../effort";
import { useT3 } from "./attention";

// Starts one thread per item on the chosen instance. Runs on an explicit click only.
export function LaunchDialog({ items, role, onClose }: { items: Item[]; role: "triage" | "implementation"; onClose: () => void }) {
  const qc = useQueryClient();
  const t3 = useT3();
  const instances = t3.data?.filter((s) => s.state === "available") ?? [];
  const [instance, setInstance] = useState("");
  const instanceId = instance || instances[0]?.instance_id || "";
  const catalog = useQuery({ queryKey: ["t3-catalog", instanceId], queryFn: () => api.t3Catalog(instanceId), enabled: !!instanceId });
  const [project, setProject] = useState("");
  const [choice, setChoice] = useState("");
  const [worktree, setWorktree] = useState(role === "implementation");
  const [base, setBase] = useState("");
  const firstProvider = catalog.data?.providers[0];
  const selected = choice || (firstProvider ? `${firstProvider.instance_id}|${firstProvider.models[0].slug}` : "");
  const [provider0, model0] = selected.split("|");
  const model = catalog.data?.providers.find((p) => p.instance_id === provider0)?.models.find((m) => m.slug === model0);
  const [effort, setEffort] = useState("");
  const effortValue = effort || (model?.effort ? savedEffort(model.slug, model.effort.values) || model.effort.default : "");
  // The item's Dory project gives the repository, which picks the T3 project and the branches.
  const repos = useQuery({ queryKey: ["repositories", items[0]?.project_id], queryFn: () => api.repositories.list({ project_id: items[0]?.project_id }), enabled: !!items[0]?.project_id });
  const repo = repos.data?.[0];
  const repoPath = repo?.remote_url.match(/github\.com[:/]([^/]+\/[^/]+?)(?:\.git)?\/?$/)?.[1] ?? "";
  const repoName = repoPath.split("/")[1]?.toLowerCase() ?? "";
  const matched = repoName ? catalog.data?.projects.find((p) => p.root.toLowerCase().replace(/\/+$/, "").split("/").pop() === repoName || p.title.toLowerCase() === repoName) : undefined;
  const projectId = project || matched?.id || catalog.data?.projects[0]?.id || "";
  const branches = useQuery({ queryKey: ["branches", repoPath], queryFn: () => api.branches(repoPath), enabled: worktree && !!repoPath });
  const baseValue = base || repo?.default_branch || "";
  const branchOptions = [...new Set([repo?.default_branch, ...(branches.data ?? [])].filter((b): b is string => !!b))];
  const launch = useMutation({
    mutationFn: async () => {
      const [provider, slug] = selected.split("|");
      for (const item of items) await api.t3Launch(item.id, { instance_id: instanceId, project_id: projectId, provider, model: slug, role, worktree, base_branch: baseValue, effort_option: model?.effort?.id, effort: model?.effort ? effortValue : undefined });
    },
    onSuccess: () => {
      if (model?.effort) saveEffort(model.slug, effortValue);
      qc.invalidateQueries({ queryKey: ["items"] });
      qc.invalidateQueries({ queryKey: ["item-threads"] });
      onClose();
    },
  });
  return (
    <Modal title={role === "triage" ? `Analizar ${items.length} con un agente` : "Enviar a implementar"} onClose={onClose}>
      <Label text="Máquina">
        <Select value={instanceId} onChange={(v) => { setInstance(v); setProject(""); setChoice(""); }} options={instances.map((s) => ({ value: s.instance_id, label: s.base_url }))} />
      </Label>
      <Label text="Proyecto de T3">
        <Select value={projectId} onChange={setProject} options={(catalog.data?.projects ?? []).map((p) => ({ value: p.id, label: p.title }))} />
      </Label>
      <Label text="Modelo">
        <Select value={selected} onChange={(v) => { setChoice(v); setEffort(""); }} options={(catalog.data?.providers ?? []).flatMap((p) => p.models.map((m) => ({ value: `${p.instance_id}|${m.slug}`, label: m.name, group: p.name })))} />
      </Label>
      {model?.effort && (
        <Label text="Razonamiento">
          <Select value={effortValue} onChange={setEffort} options={model.effort.values.map((v) => ({ value: v, label: v }))} />
        </Label>
      )}
      <label className="flex items-center gap-2 text-sm"><input type="checkbox" checked={worktree} onChange={(e) => setWorktree(e.target.checked)} />Crear un worktree nuevo</label>
      {worktree && (
        <Label text="Rama base">
          {branchOptions.length ? <Select value={baseValue} onChange={setBase} options={branchOptions.map((b) => ({ value: b, label: b === repo?.default_branch ? `${b} (por defecto)` : b }))} />
            : <input value={base} onChange={(e) => setBase(e.target.value)} placeholder="Rama por defecto del repositorio" className="w-full rounded border border-slate-300 bg-transparent px-2 py-1 text-sm dark:border-slate-700" />}
        </Label>
      )}
      <p className="text-xs text-slate-500 dark:text-slate-400">El agente se lanza con acceso completo.</p>
      <ErrorText error={catalog.error ?? launch.error} />
      <Button disabled={!selected || !projectId || launch.isPending} onClick={() => launch.mutate()}>Lanzar</Button>
    </Modal>
  );
}
