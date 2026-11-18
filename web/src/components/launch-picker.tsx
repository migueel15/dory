import { useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { api, type LaunchChoice, type Model } from "../api/client";
import { Label, Select } from "./ui";
import { explicitModels, findModel, lastModel, modelEffort, modelId, rememberModel } from "../effort";
import { ModelLogo } from "./model-logo";

export const ProviderIcon = ({ driver, model }: { driver: string; model?: string }) => <ModelLogo driver={driver} model={model} />;

const KEY = "dory.launch";
const stored = (section: string): Partial<LaunchChoice> => {
  try { return JSON.parse(localStorage.getItem(`${KEY}.${section}`) ?? "{}"); } catch { return {}; }
};

// useProviders lists the harnesses installed on this node.
export function useProviders() {
  return useQuery({ queryKey: ["node"], queryFn: api.node, staleTime: 60_000, select: (n) => n.providers.filter((p) => p.installed) });
}

// useLaunchForm picks provider, model, effort and access for a new thread.
// The last selected model is shared across launch forms. Permissions stay per section.
export function useLaunchForm({ section }: { section: string; repo?: string }) {
  const providers = useProviders();
  const [draft, setDraft] = useState<Partial<LaunchChoice>>(() => ({ runtime_mode: stored(section).runtime_mode }));
  const last = lastModel() ?? stored(section);
  const list = (providers.data ?? []).map((p) => ({ ...p, models: explicitModels(p.models) })).filter((p) => p.models.length > 0);
  const provider = list.find((p) => p.id === (draft.provider ?? last.provider)) ?? list[0];
  const model = provider && (findModel(provider.models, draft.model ?? last.model ?? "") ?? provider.models[0]);
  const draftEffort = draft.effort ?? "";
  const value: LaunchChoice = {
    provider: provider?.id ?? "",
    model: model ? modelId(model) : "",
    effort: model && provider
      ? (draft.model === modelId(model) && model.efforts?.includes(draftEffort) ? draftEffort : modelEffort(provider.id, model))
      : "",
    runtime_mode: draft.runtime_mode ?? "full-access",
  };
  const setRuntimeMode = (runtime_mode: LaunchChoice["runtime_mode"]) =>
    setDraft((current) => ({ ...current, runtime_mode }));
  const selectModel = (providerId: string, selected: Model, effort = modelEffort(providerId, selected)) => {
    const choice = { provider: providerId, model: modelId(selected), effort };
    rememberModel(choice);
    setDraft((current) => ({ ...current, ...choice }));
  };
  const fields = (
    <div className="grid gap-2 sm:grid-cols-2">
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
      <Label text="Permisos">
        <Select value={value.runtime_mode} onChange={(v) => setRuntimeMode(v as LaunchChoice["runtime_mode"])}
          options={[{ value: "full-access", label: "Acceso completo" }, { value: "approval-required", label: "Solo ediciones" }]} />
      </Label>
    </div>
  );
  return {
    fields, value, selectModel, setRuntimeMode, providers: list, ready: !!model, error: providers.error,
    remember: () => { try { localStorage.setItem(`${KEY}.${section}`, JSON.stringify({ ...stored(section), runtime_mode: value.runtime_mode })); } catch { /* per browser only */ } },
  };
}
