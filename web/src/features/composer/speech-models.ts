import { useSyncExternalStore } from "react";
import type { SpeechCatalog } from "../../api/queries/dictation";
import { storage } from "../../lib/storage";
import { events } from "../../lib/store";
import { modelKey,useInputSettingsSync } from "../settings/input-settings";

export { modelsPath,speechKey,useSpeechModels,type SpeechCatalog,type SpeechModel } from "../../api/queries/dictation";
export const preferenceKey = modelKey;


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
