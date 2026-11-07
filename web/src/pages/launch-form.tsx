import { ModelLogo } from "../components/model-logo";
import { useState, type ReactNode } from "react";
import { useQueries, useQuery } from "@tanstack/react-query";
import { api } from "../api/client";
import { Input, Label, Select } from "../components/ui";
import { saveEffort, savedEffort } from "../effort";
import { saveModel, savedModel } from "../last-model";
import { useT3 } from "./attention";

// ProviderIcon marks the company behind a model with its logo.
export const ProviderIcon = ({ driver, model }: { driver: string; model?: string }) => <ModelLogo driver={driver} model={model} />;

const stateLabel: Record<string, string> = { available: "disponible", disconnected: "desconectado", auth_failed: "sin acceso", incompatible: "incompatible" };
const stateDot = (s: string) => (s === "available" ? "bg-green-500" : s === "disconnected" ? "bg-slate-400" : "bg-red-500");
// repoKey reduces a remote or web URL to host/owner/name, as the backend does.
export const repoKey = (raw: string) => raw.trim().toLowerCase().replace(/^(https?|ssh|git):\/\//, "").replace(/^[^@/:]+@/, "").replace(":", "/").replace(/\/+$/, "").replace(/\.git$/, "");
const repoOf = (remote: string) => remote.match(/github\.com[:/]([^/]+\/[^/]+?)(?:\.git)?\/?$/)?.[1] ?? "";

export type LaunchChoice = { instance_id: string; project_id: string; provider: string; model: string; effort_option?: string; effort?: string; worktree?: boolean; base_branch?: string };

// useLaunchForm is the one form every T3 thread launch uses: machine, T3 project,
// model, reasoning and, when the launch can work in a worktree, its base branch.
// The repository comes from a Dory project or an owner/repo path.
export function useLaunchForm({ section, doryProject, repo: repoPathIn, worktree: worktreeDefault }: { section: string; doryProject?: string | null; repo?: string; worktree?: boolean }) {
  const t3 = useT3();
  const envs = useQuery({ queryKey: ["environments"], queryFn: () => api.environments.list() });
  const envName = (id: string) => envs.data?.find((e) => e.id === id)?.name ?? "";
  const repos = useQuery({ queryKey: ["repositories", doryProject], queryFn: () => api.repositories.list({ project_id: doryProject }), enabled: !!doryProject && !repoPathIn });
  const all = useQuery({ queryKey: ["repositories"], queryFn: () => api.repositories.list(), enabled: !!repoPathIn });
  const repo = repoPathIn ? all.data?.find((r) => repoOf(r.remote_url).toLowerCase() === repoPathIn.toLowerCase()) : repos.data?.[0];
  const repoPath = repoPathIn || (repo ? repoOf(repo.remote_url) : "");
  // The repository the work is about; a machine qualifies only if one of its T3 projects has it.
  const wanted = new Set([...(repoPathIn ? [`github.com/${repoPathIn.toLowerCase()}`] : []), ...(repoPathIn ? [] : (repos.data ?? []).map((r) => repoKey(r.remote_url)))]);
  if (repo) wanted.add(repoKey(repo.remote_url));
  const restricted = wanted.size > 0;

  // Machines in name order, so the list never reshuffles between reloads.
  const statuses = [...(t3.data ?? [])].sort((a, b) => (envName(a.environment_id) || a.base_url).localeCompare(envName(b.environment_id) || b.base_url, "es", { sensitivity: "base" }));
  const catalogs = useQueries({ queries: statuses.map((s) => ({ queryKey: ["t3-catalog", s.instance_id], queryFn: () => api.t3Catalog(s.instance_id), enabled: s.state === "available", staleTime: 60_000 })) });
  const catalogOf = (id: string) => catalogs[statuses.findIndex((s) => s.instance_id === id)]?.data;
  const projectsFor = (id: string) => (catalogOf(id)?.projects ?? []).filter((p) => !restricted || (p.repo && wanted.has(repoKey(p.repo))));
  const eligible = (id: string) => !restricted || projectsFor(id).length > 0;
  const available = statuses.filter((s) => s.state === "available" && eligible(s.instance_id));
  // infra-admin is the default machine: the server that runs the agents.
  const fallback = available.find((s) => /infra/i.test(envName(s.environment_id))) ?? available[0];
  const [instance, setInstance] = useState("");
  const instanceId = (instance && eligible(instance) ? instance : "") || fallback?.instance_id || "";
  const catalog = { data: catalogOf(instanceId), error: catalogs[statuses.findIndex((s) => s.instance_id === instanceId)]?.error ?? null };
  const checking = restricted && catalogs.some((c) => c.isPending && c.fetchStatus !== "idle");

  const choices = projectsFor(instanceId);
  const [project, setProject] = useState("");
  const projectId = (project && choices.some((p) => p.id === project) ? project : "") || (restricted ? choices[0]?.id : "") || "";

  const [choice, setChoice] = useState("");
  const first = catalog.data?.providers[0];
  // The last model used in this section, if this machine has it (same slug under another provider counts).
  const [lastProvider, lastSlug] = savedModel(section).split("|");
  const last = catalog.data?.providers.find((p) => p.instance_id === lastProvider && p.models.some((m) => m.slug === lastSlug))
    ?? catalog.data?.providers.find((p) => p.models.some((m) => m.slug === lastSlug));
  const selected = choice || (last && lastSlug ? `${last.instance_id}|${lastSlug}` : "") || (first?.models[0] ? `${first.instance_id}|${first.models[0].slug}` : "");
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
        <Select value={instanceId} onChange={(v) => { setInstance(v); setProject(""); setChoice(""); setEffort(""); }} placeholder={checking ? "Buscando máquinas con el repositorio…" : restricted ? "Ninguna máquina tiene este repositorio" : "—"} options={statuses.map((s) => ({
          value: s.instance_id,
          label: `${envName(s.environment_id) || s.base_url}${s.state !== "available" ? ` (${stateLabel[s.state] ?? s.state})` : !eligible(s.instance_id) ? " (sin este repositorio)" : ""}`,
          disabled: s.state !== "available" || !eligible(s.instance_id),
          icon: <span title={stateLabel[s.state] ?? s.state} className={`size-2 shrink-0 rounded-full ${stateDot(s.state)}`} />,
        }))} />
      </Label>
      <Label text="Proyecto de T3">
        <Select value={projectId} onChange={setProject} placeholder="Elige el proyecto del repositorio" options={choices.map((p) => ({ value: p.id, label: p.title }))} />
      </Label>
      <Label text="Modelo">
        <Select value={selected} onChange={(v) => { setChoice(v); setEffort(""); }} options={(catalog.data?.providers ?? []).flatMap((p) => p.models.map((m) => ({ value: `${p.instance_id}|${m.slug}`, label: m.name, group: p.name, icon: <ProviderIcon driver={p.driver || p.name} model={`${m.slug} ${m.name}`} /> })))} />
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
            : <Input value={base} onChange={(e) => setBase(e.target.value)} placeholder="Rama por defecto del repositorio" />}
        </Label>
      )}
    </>
  );
  return {
    fields, value, error: catalog.error,
    ready: !!instanceId && !!projectId && !!model,
    // remember keeps the reasoning used with this model as its next default.
    remember: () => { if (model?.effort) saveEffort(model.slug, effortValue); if (selected) saveModel(section, selected); },
  };
}
