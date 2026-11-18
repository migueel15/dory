import { useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { api, usable, type LaunchChoice, type Model } from "../api/client";
import { Label, Select } from "./ui";
import { explicitModels, findModel, lastModel, modelEffort, modelId, rememberModel } from "../effort";
import { ModelLogo } from "./model-logo";

export const ProviderIcon = ({ driver, model }: { driver: string; model?: string }) => <ModelLogo driver={driver} model={model} />;

const KEY = "dory.launch";
const stored = (section: string): Partial<LaunchChoice> => {
  try { return JSON.parse(localStorage.getItem(`${KEY}.${section}`) ?? "{}"); } catch { return {}; }
};

// useProviders lists the harnesses installed on a node, this one by default.
export function useProviders(node = "local") {
  const local = useQuery({ queryKey: ["node"], queryFn: api.node, staleTime: 60_000, select: (n) => n.providers.filter(usable), enabled: node === "local" });
  const remote = useQuery({ queryKey: ["node-providers", node], queryFn: () => api.nodeProviders(node), staleTime: 60_000, select: (ps) => ps.filter(usable), enabled: node !== "local" });
  return node === "local" ? local : remote;
}

// useNodes lists where a new thread can run: this node and the connected peers.
export function useNodes() {
  const status = useQuery({ queryKey: ["node"], queryFn: api.node, staleTime: 60_000 });
  const peers = (status.data?.peers ?? []).filter((p) => p.connected && !p.revoked);
  return [{ value: "local", label: status.data?.node.name ?? "Este nodo" }, ...peers.map((p) => ({ value: p.id, label: p.name }))];
}

// useProjectNodes lists the nodes that keep a repository of a project, with
// this node as "local". Empty when the project has no known environment.
export function useProjectNodes(project?: string | null) {
  const status = useQuery({ queryKey: ["node"], queryFn: api.node, staleTime: 60_000 });
  const repos = useQuery({ queryKey: ["repositories"], queryFn: () => api.repositories.list(), enabled: !!project });
  const locations = useQuery({ queryKey: ["repository-locations"], queryFn: api.repositoryLocations, enabled: !!project });
  const ids = new Set((repos.data ?? []).filter((r) => r.project_id === project).map((r) => r.id));
  const self = status.data?.node.id;
  return [...new Set((locations.data ?? []).filter((l) => ids.has(l.repository_id)).map((l) => (l.node_id === self ? "local" : l.node_id)))];
}

// useLaunchForm picks provider, model, effort and access for a new thread.
// The last selected model is shared across launch forms. Permissions stay per section.
// With a project, only the environments that keep its repository are offered.
export function useLaunchForm({ section, project }: { section: string; repo?: string; project?: string | null }) {
  const [picked, setNode] = useState("local");
  const all = useNodes();
  const homes = useProjectNodes(project);
  const nodes = homes.length ? all.filter((n) => homes.includes(n.value)) : all;
  const node = nodes.some((n) => n.value === picked) ? picked : nodes[0]?.value ?? "local";
  const providers = useProviders(node);
  const [draft, setDraft] = useState<Partial<LaunchChoice>>({});
  const last = lastModel() ?? stored(section);
  const list = (providers.data ?? []).map((p) => ({ ...p, models: explicitModels(p.models) })).filter((p) => p.models.length > 0);
  const provider = list.find((p) => p.id === (draft.provider ?? last.provider)) ?? list[0];
  const model = provider && (findModel(provider.models, draft.model ?? last.model ?? "") ?? provider.models[0]);
  const draftEffort = draft.effort ?? "";
  const value: LaunchChoice = {
    node,
    provider: provider?.id ?? "",
    model: model ? modelId(model) : "",
    effort: model && provider
      ? (draft.model === modelId(model) && model.efforts?.includes(draftEffort) ? draftEffort : modelEffort(provider.id, model))
      : "",
    runtime_mode: "full-access",
  };
  const selectModel = (providerId: string, selected: Model, effort = modelEffort(providerId, selected)) => {
    const choice = { provider: providerId, model: modelId(selected), effort };
    rememberModel(choice);
    setDraft((current) => ({ ...current, ...choice }));
  };
  const fields = (
    <div className="grid gap-2 sm:grid-cols-2">
      {nodes.length > 1 && (
        <Label text="Nodo">
          <Select value={node} onChange={setNode} options={nodes} />
        </Label>
      )}
      <Label text="Agente">
        <Select value={value.provider} onChange={(v) => { const first = list.find((p) => p.id === v)?.models[0]; if (first) selectModel(v, first); }}
          options={list.map((p) => ({ value: p.id, label: `${p.name}${p.version ? ` · ${p.version.split(" ")[0]}` : ""}`, icon: <ProviderIcon driver={p.id} /> }))} />
      </Label>
      <Label text="Modelo">
        <Select value={value.model} onChange={(v) => { const selected = findModel(provider?.models ?? [], v); if (selected) selectModel(value.provider, selected); }} options={(provider?.models ?? []).map((m) => ({ value: modelId(m), label: m.name }))} />
      </Label>
      {!!model?.efforts?.length && (
        <Label text="Razonamiento">
          <Select value={value.effort} onChange={(v) => selectModel(value.provider, model, v)} options={model.efforts.map((e) => ({ value: e, label: e }))} />
        </Label>
      )}
    </div>
  );
  return {
    fields, value, nodes, setNode, selectModel, providers: list, ready: !!model, error: providers.error,
    remember: () => { try { localStorage.setItem(`${KEY}.${section}`, JSON.stringify({ ...stored(section), runtime_mode: value.runtime_mode })); } catch { /* per browser only */ } },
  };
}
