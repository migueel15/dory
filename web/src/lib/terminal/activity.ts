import { useSyncExternalStore } from "react";

// Tabs of terminals with output the user has not looked at yet, per thread.
const unseen = new Map<string, Set<string>>();
const listeners = new Set<() => void>();
let version = 0;
const emit = () => { version++; listeners.forEach((l) => l()); };
const keyOf = (threadId: string, tab: string) => `${threadId}|${tab}`;

export function markUnseen(threadId: string, tab: string) {
  const k = keyOf(threadId, tab);
  if (unseen.has(k)) return;
  unseen.set(k, new Set());
  emit();
}

export function markSeen(threadId: string, tab: string) {
  if (unseen.delete(keyOf(threadId, tab))) emit();
}

/** useHasUnseen tells whether a terminal tab has output nobody has seen. */
export function useHasUnseen(threadId: string, tab: string): boolean {
  useSyncExternalStore((l) => { listeners.add(l); return () => listeners.delete(l); }, () => version);
  return unseen.has(keyOf(threadId, tab));
}
