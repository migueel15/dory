import type { Model } from "./api/client";

type ModelChoice = { provider: string; model?: string; effort?: string };
const lastModelKey = "dory.last-model";
const effortKey = (provider: string, model: string) => `effort:${provider}:${model}`;

export const modelId = (model: Model) => model.resolved_model || model.slug;

export function explicitModels(models: Model[]) {
  const explicit = new Map<string, Model>();
  for (const model of models) {
    const id = modelId(model);
    if (model.slug && model.slug !== "default" && !explicit.has(id)) explicit.set(id, model);
  }
  return [...explicit.values()];
}

export function findModel(models: Model[], id: string) {
  return models.find((model) => model.slug === id || modelId(model) === id);
}

export function lastModel(): ModelChoice | undefined {
  try {
    return JSON.parse(localStorage.getItem(lastModelKey) ?? "null") ?? undefined;
  } catch {
    return undefined;
  }
}

export function modelEffort(provider: string, model: Model) {
  const values = model.efforts ?? [];
  let saved = "";
  try {
    saved = localStorage.getItem(effortKey(provider, modelId(model))) ??
      localStorage.getItem(`effort:${model.slug}`) ?? "";
  } catch {
    // Storage can be unavailable in private browsing.
  }
  return values.includes(saved) ? saved :
    values.includes("medium") ? "medium" :
    values.includes("low") ? "low" : values[0] ?? "";
}

export function rememberModel({ provider, model, effort }: ModelChoice) {
  if (!model || model === "default") return;
  try {
    localStorage.setItem(lastModelKey, JSON.stringify({ provider, model }));
    if (effort) localStorage.setItem(effortKey(provider, model), effort);
  } catch {
    // Storage can be unavailable in private browsing.
  }
}
