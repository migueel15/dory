import { useSyncExternalStore } from "react";
import { useQuery } from "@tanstack/react-query";
import { api } from "./api/client";

// The selected project is global app state: every page filters by it.
// "" means all projects. It is remembered per browser in localStorage.
const KEY = "project";
const listeners = new Set<() => void>();
let current = (() => { try { return localStorage.getItem(KEY) ?? ""; } catch { return ""; } })();

export function setProject(id: string) {
  current = id;
  try { if (id) localStorage.setItem(KEY, id); else localStorage.removeItem(KEY); } catch { /* storage unavailable */ }
  listeners.forEach((l) => l());
}

const subscribe = (l: () => void) => { listeners.add(l); return () => listeners.delete(l); };
export const useProject = () => useSyncExternalStore(subscribe, () => current);

// useProjectThreads returns the T3 thread keys ("instance:thread") linked to the
// project's items, or null when no project is selected (no filtering).
export function useProjectThreads(project: string) {
  const links = useQuery({ queryKey: ["item-threads"], queryFn: () => api.itemThreads.list(), enabled: !!project });
  const items = useQuery({ queryKey: ["items", "project", project], queryFn: () => api.items.list({ project_id: project }), enabled: !!project });
  const ids = new Set(items.data?.map((i) => i.id));
  const keys = project ? new Set(links.data?.filter((l) => ids.has(l.item_id)).map((l) => `${l.instance_id}:${l.thread_id}`)) : null;
  return { keys, error: links.error ?? items.error, pending: !!project && (links.isPending || items.isPending) };
}
