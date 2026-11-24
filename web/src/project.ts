import { useSyncExternalStore } from "react";
import { storage, storageKeys } from "./lib/storage";

// The selected project is global app state: every page filters by it.
// "" means all projects. It is remembered per browser in localStorage.
const KEY = storageKeys.project;
const listeners = new Set<() => void>();
let current = (() => { try { return storage.get(KEY) ?? ""; } catch { return ""; } })();

export function setProject(id: string) {
  current = id;
  try { if (id) storage.set(KEY, id); else storage.remove(KEY); } catch { /* storage unavailable */ }
  listeners.forEach((l) => l());
}

const subscribe = (l: () => void) => { listeners.add(l); return () => listeners.delete(l); };
export const useProject = () => useSyncExternalStore(subscribe, () => current);

