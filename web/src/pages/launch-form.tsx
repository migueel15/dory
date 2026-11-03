import { useState, type ReactNode } from "react";
import { useQuery } from "@tanstack/react-query";
import { api } from "../api/client";
import { Label, Select } from "../components/ui";
import { saveEffort, savedEffort } from "../effort";
import { useT3 } from "./attention";

// ProviderIcon marks the company behind a model with its initial and colour.
export function ProviderIcon({ driver }: { driver: string }) {
  const d = driver.toLowerCase();
  const [text, color] = d.includes("claude") ? ["A", "bg-orange-600"] : d.includes("codex") || d.includes("openai") ? ["O", "bg-emerald-700"] : d.includes("gemini") ? ["G", "bg-blue-600 dark:bg-accent"] : [driver.slice(0, 1).toUpperCase() || "?", "bg-slate-500"];
  return <span className={`grid size-5 shrink-0 place-items-center rounded text-[10px] font-bold text-white ${color}`}>{text}</span>;
}

const stateLabel: Record<string, string> = { available: "disponible", disconnected: "desconectado", auth_failed: "sin acceso", incompatible: "incompatible" };
const stateDot = (s: string) => (s === "available" ? "bg-green-500" : s === "disconnected" ? "bg-slate-400" : "bg-red-500");
const repoOf = (remote: string) => remote.match(/github\.com[:/]([^/]+\/[^/]+?)(?:\.git)?\/?$/)?.[1] ?? "";

export type LaunchChoice = { instance_id: string; project_id: string; provider: string; model: string; effort_option?: string; effort?: string; worktree?: boolean; base_branch?: string };

// useLaunchForm is the one form every T3 thread launch uses: machine, T3 project,
// model, reasoning and, when the launch can work in a worktree, its base branch.
// The repository comes from a Dory project or an owner/repo path.
export function useLaunchForm({ doryProject, repo: repoPathIn, worktree: worktreeDefault }: { doryProject?: string | null; repo?: string; worktree?: boolean }) {
  const t3 = useT3();
  const envs = useQuery({ queryKey: ["environments"], queryFn: () => api.environments.list() });
  const envName = (id: string) => envs.data?.find((e) => e.id === id)?.name ?? "";
  const available = t3.data?.filter((s) => s.state === "available") ?? [];
  // infra-admin is the default machine: the server that runs the agents.
  const fallback = available.find((s) => /infra/i.test(envName(s.environment_id))) ?? available[0];
  const [instance, setInstance] = useState("");
  const instanceId = instance || fallback?.instance_id || "";
  const catalog = useQuery({ queryKey: ["t3-catalog", instanceId], queryFn: () => api.t3Catalog(instanceId), enabled: !!instanceId });

  const repos = useQuery({ queryKey: ["repositories", doryProject], queryFn: () => api.repositories.list({ project_id: doryProject }), enabled: !!doryProject && !repoPathIn });
  const all = useQuery({ queryKey: ["repositories"], queryFn: () => api.repositories.list(), enabled: !!repoPathIn });
  const repo = repoPathIn ? all.data?.find((r) => repoOf(r.remote_url).toLowerCase() === repoPathIn.toLowerCase()) : repos.data?.[0];
  const repoPath = repoPathIn || (repo ? repoOf(repo.remote_url) : "");
  const repoName = repoPath.split("/")[1]?.toLowerCase() ?? "";
  const matched = repoName ? catalog.data?.projects.find((p) => p.root.toLowerCase().replace(/\/+$/, "").split("/").pop() === repoName || p.title.toLowerCase() === repoName) : undefined;
  const [project, setProject] = useState("");
  const projectId = project || matched?.id || "";

  const [choice, setChoice] = useState("");
  const first = catalog.data?.providers[0];
  const selected = choice || (first?.models[0] ? `${first.instance_id}|${first.models[0].slug}` : "");
  const [providerId, slug] = selected.split("|");
  const provider = catalog.data?.providers.find((p) => p.instance_id === providerId);
  const model = provider?.models.find((m) => m.slug === slug);
  const [effort, setEffort] = useState("");
  const effortValue = effort || (model?.effort ? savedEffort(model.slug, model.effort.values) || model.effort.default : "");

  const canWorktree = worktreeDefault !== undefined;
  const [worktree, setWorktree] = useState(!!worktreeDefault);
  const [base, setBase] = useState("");
  const branches = useQuery({ queryKey: ["branches", repoPath], queryFn: () => api.branches(repoPath), enabled: canWorktree && worktree && !!repoPath });
  const baseValue = base || repo?.default_branch || "";
  const branchOptions = [...new Set([repo?.default_branch, ...(branches.data ?? [])].filter((b): b is string => !!b))];

  const value: LaunchChoice = {
    instance_id: instanceId, project_id: projectId, provider: providerId ?? "", model: slug ?? "",
    effort_option: model?.effort?.id, effort: model?.effort ? effortValue : undefined,
    ...(canWorktree ? { worktree, base_branch: worktree ? baseValue : undefined } : {}),
  };
  const fields: ReactNode = (
    <>
      <Label text="Máquina">
        <Select value={instanceId} onChange={(v) => { setInstance(v); setProject(""); setChoice(""); setEffort(""); }} options={(t3.data ?? []).map((s) => ({
          value: s.instance_id,
          label: `${envName(s.environment_id) || s.base_url}${s.state === "available" ? "" : ` (${stateLabel[s.state] ?? s.state})`}`,
          disabled: s.state !== "available",
          icon: <span title={stateLabel[s.state] ?? s.state} className={`size-2 shrink-0 rounded-full ${stateDot(s.state)}`} />,
        }))} />
      </Label>
      <Label text="Proyecto de T3">
        <Select value={projectId} onChange={setProject} placeholder="Elige el proyecto del repositorio" options={(catalog.data?.projects ?? []).map((p) => ({ value: p.id, label: p.title }))} />
      </Label>
      <Label text="Modelo">
        <Select value={selected} onChange={(v) => { setChoice(v); setEffort(""); }} options={(catalog.data?.providers ?? []).flatMap((p) => p.models.map((m) => ({ value: `${p.instance_id}|${m.slug}`, label: m.name, group: p.name, icon: <ProviderIcon driver={p.driver || p.name} /> })))} />
      </Label>
      {model?.effort && (
        <Label text="Razonamiento">
          <Select value={effortValue} onChange={setEffort} options={model.effort.values.map((v) => ({ value: v, label: v }))} />
        </Label>
      )}
      {canWorktree && <label className="flex items-center gap-2 text-sm"><input type="checkbox" checked={worktree} onChange={(e) => setWorktree(e.target.checked)} />Crear un worktree nuevo</label>}
      {canWorktree && worktree && (
        <Label text="Rama base">
          {branchOptions.length ? <Select value={baseValue} onChange={setBase} options={branchOptions.map((b) => ({ value: b, label: b === repo?.default_branch ? `${b} (por defecto)` : b }))} />
            : <input value={base} onChange={(e) => setBase(e.target.value)} placeholder="Rama por defecto del repositorio" className="w-full rounded border border-slate-300 bg-transparent px-2 py-1 text-sm dark:border-border" />}
        </Label>
      )}
    </>
  );
  return {
    fields, value, error: catalog.error,
    ready: !!instanceId && !!projectId && !!model,
    // remember keeps the reasoning used with this model as its next default.
    remember: () => { if (model?.effort) saveEffort(model.slug, effortValue); },
  };
}
