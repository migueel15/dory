import { useEffect, useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { api, type LaunchChoice, type Provider } from "../api/client";
import { Label, Select } from "./ui";
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
// The last choice of each section is remembered in this browser.
export function useLaunchForm({ section }: { section: string; repo?: string }) {
  const providers = useProviders();
  const saved = stored(section);
  const [value, setValue] = useState<LaunchChoice>({ provider: saved.provider ?? "", model: saved.model ?? "", effort: saved.effort ?? "", runtime_mode: saved.runtime_mode ?? "full-access" });
  const list: Provider[] = providers.data ?? [];
  const provider = list.find((p) => p.id === value.provider);
  // Fall back to the first installed provider and its first model.
  useEffect(() => {
    if (!list.length) return;
    if (!provider) setValue((v) => ({ ...v, provider: list[0].id, model: list[0].models[0]?.slug ?? "", effort: "" }));
  }, [list.length, provider]); // eslint-disable-line react-hooks/exhaustive-deps
  const model = provider?.models.find((m) => m.slug === value.model);
  const fields = (
    <div className="grid gap-2 sm:grid-cols-2">
      <Label text="Agente">
        <Select value={value.provider} onChange={(v) => { const p = list.find((x) => x.id === v); setValue((c) => ({ ...c, provider: v, model: p?.models[0]?.slug ?? "", effort: "" })); }}
          options={list.map((p) => ({ value: p.id, label: `${p.name}${p.version ? ` · ${p.version.split(" ")[0]}` : ""}`, icon: <ProviderIcon driver={p.id} /> }))} />
      </Label>
      <Label text="Modelo">
        <Select value={value.model} onChange={(v) => setValue((c) => ({ ...c, model: v, effort: "" }))} options={(provider?.models ?? []).map((m) => ({ value: m.slug, label: m.name }))} />
      </Label>
      {!!model?.efforts?.length && (
        <Label text="Razonamiento">
          <Select value={value.effort} onChange={(v) => setValue((c) => ({ ...c, effort: v }))} options={[{ value: "", label: "Por defecto" }, ...model.efforts.map((e) => ({ value: e, label: e }))]} />
        </Label>
      )}
      <Label text="Permisos">
        <Select value={value.runtime_mode} onChange={(v) => setValue((c) => ({ ...c, runtime_mode: v as LaunchChoice["runtime_mode"] }))}
          options={[{ value: "full-access", label: "Acceso completo" }, { value: "approval-required", label: "Solo ediciones" }]} />
      </Label>
    </div>
  );
  return {
    fields, value, ready: !!value.provider, error: providers.error,
    remember: () => { try { localStorage.setItem(`${KEY}.${section}`, JSON.stringify(value)); } catch { /* per browser only */ } },
  };
}
