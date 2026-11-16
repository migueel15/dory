import { useSyncExternalStore } from "react";

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

