// Hooks of the suggestion menu of the composer: commands, skills and files.
import { useQuery } from "@tanstack/react-query";
import type { Opts } from "./hooks";
import { qk } from "./keys";

export type CatalogEntry = { name: string; description: string; insert: string };

// SuggestSource is where the menu reads from: a thread, or the harness and
// project of a thread about to be created.
export type SuggestSource = { key: unknown[]; commands: () => Promise<{ commands: CatalogEntry[]; skills: CatalogEntry[] }>; files?: (q: string) => Promise<string[]> };

export const useCommandCatalog = (source: SuggestSource, o?: Opts<Awaited<ReturnType<SuggestSource["commands"]>>>) =>
  useQuery({ queryKey: qk.commands(...source.key), queryFn: source.commands, ...o });
export const useFileSuggestions = (source: SuggestSource, q: string | undefined, o?: Opts<string[]>) =>
  useQuery({ queryKey: qk.files(...source.key, q), queryFn: () => source.files!(q!), ...o });
