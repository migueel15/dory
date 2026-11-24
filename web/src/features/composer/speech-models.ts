import { useSyncExternalStore } from "react";
import { useQuery } from "@tanstack/react-query";
import { req } from "../../api/client";
import { modelKey, useInputSettingsSync } from "../settings/input-settings";
import { qk } from "../../api/queries/keys";
import { events } from "../../lib/store";
import { storage } from "../../lib/storage";

export type SpeechModel = {
  id: string; name: string; bytes: number; streaming: boolean;
  status: "available" | "downloading" | "installed" | "failed"; error?: string;
};
export type SpeechCatalog = { runtime: boolean; models: SpeechModel[] };

export const modelsPath = "/dictation/models";
export const speechKey = qk.speechModels();
export const preferenceKey = modelKey;

export function useSpeechModels() {
  return useQuery({
    queryKey: speechKey, queryFn: () => req<SpeechCatalog>("GET", modelsPath),
    refetchInterval: (query) => query.state.data?.models.some((model) => model.status === "downloading") ? 1000 : false,
  });
}

export function subscribe(notify: () => void) {
  const off = events.dictationModelChanged.on(notify);
  window.addEventListener("storage", notify);
  return () => { off(); window.removeEventListener("storage", notify); };
}

// pickSpeechModel is the model a node uses: the chosen one if it is installed,
// otherwise the best installed one.
export function pickSpeechModel(catalog: SpeechCatalog | undefined, id: string | null | undefined) {
  if (!catalog?.runtime) return undefined;
  const models = catalog.models.filter((model) => model.status === "installed");
  return models.find((model) => model.id === id) ?? models.find((model) => model.streaming) ?? models[0];
}

// useSpeechModel is the model of this node, kept in sync with its settings.
export function useSpeechModel(catalog?: SpeechCatalog) {
  useInputSettingsSync();
  const id = useSyncExternalStore(subscribe, () => {
    try { return storage.get(preferenceKey); } catch { return null; }
  });
  return pickSpeechModel(catalog, id);
}

export const downloadSpeechModel = (id: string) => req<SpeechCatalog>("POST", `${modelsPath}/${encodeURIComponent(id)}/download`);

