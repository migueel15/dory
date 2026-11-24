// Hooks of the dictation models of this node.
import { useQuery } from "@tanstack/react-query";
import { req } from "../client";
import { useCall,type MOpts } from "./hooks";
import { qk } from "./keys";

export type SpeechModel = {
  id: string; name: string; bytes: number; streaming: boolean;
  status: "available" | "downloading" | "installed" | "failed"; error?: string;
};
export type SpeechCatalog = { runtime: boolean; models: SpeechModel[] };

export const modelsPath = "/dictation/models";
export const speechKey = qk.speechModels();

// useSpeechModels lists the models, polling while one downloads.
export function useSpeechModels() {
  return useQuery({
    queryKey: speechKey, queryFn: () => req<SpeechCatalog>("GET", modelsPath),
    refetchInterval: (query) => query.state.data?.models.some((model) => model.status === "downloading") ? 1000 : false,
  });
}

export const useDownloadSpeechModel = (o?: MOpts<SpeechCatalog, string>) =>
  useCall((id: string) => req<SpeechCatalog>("POST", `${modelsPath}/${encodeURIComponent(id)}/download`), o);
