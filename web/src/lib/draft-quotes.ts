import { useSyncExternalStore } from "react";
import type { DraftQuote } from "../features/chat/quote-selection";

// The quotes waiting in each thread's message box: pieces of the chat and
// references to code from its diff. The chat and the side panel both add to
// them, so they live here, per thread, until the message is sent.
const quotes = new Map<string, DraftQuote[]>();
const listeners = new Set<() => void>();
const empty: DraftQuote[] = [];
const emit = () => listeners.forEach((l) => l());

export function addDraftQuote(threadId: string, quote: DraftQuote) {
  quotes.set(threadId, [...(quotes.get(threadId) ?? []), quote]);
  emit();
}

export function removeDraftQuotes(threadId: string, ids: string[]) {
  quotes.set(threadId, (quotes.get(threadId) ?? []).filter((q) => !ids.includes(q.id)));
  emit();
}

export function useDraftQuotes(threadId: string): DraftQuote[] {
  return useSyncExternalStore((l) => { listeners.add(l); return () => listeners.delete(l); }, () => quotes.get(threadId) ?? empty);
}
