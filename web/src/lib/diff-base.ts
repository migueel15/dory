import { useSyncExternalStore } from "react";

// The reference each thread's diff is compared with, chosen in its details
// and shared by every view of that diff. "" means the thread's base branch.
// It is remembered per viewer in this browser only.
const KEY = "dory.diff-base";
const listeners = new Set<() => void>();

function read(): Record<string, string> {
  try { return JSON.parse(localStorage.getItem(KEY) ?? "{}"); } catch { return {}; }
}

let bases = read();

export function setDiffBase(threadId: string, base: string) {
  bases = { ...bases, [threadId]: base };
  if (!base) delete bases[threadId];
  try { localStorage.setItem(KEY, JSON.stringify(bases)); } catch { /* per-viewer only */ }
  listeners.forEach((l) => l());
}

export function useDiffBase(threadId: string): string {
  return useSyncExternalStore((l) => { listeners.add(l); return () => listeners.delete(l); }, () => bases[threadId] ?? "");
}
