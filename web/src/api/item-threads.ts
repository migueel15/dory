import { useQuery } from "@tanstack/react-query";
import { api } from "./client";

type ItemThreadRole = "triage" | "implementation" | "replan" | "review" | "refine";

/** T3 threads linked to an item (optionally of one role); both queries are shared by every caller. */
export function useItemThreads(itemId: string, role?: ItemThreadRole) {
  const links = useQuery({ queryKey: ["item-threads"], queryFn: () => api.itemThreads.list(), refetchInterval: 60_000 });
  const t3 = useQuery({ queryKey: ["t3-status"], queryFn: api.t3Status, refetchInterval: 60_000 });
  return links.data?.filter((l) => l.item_id === itemId && (!role || l.role === role)).flatMap((l) => {
    const thread = t3.data?.flatMap((s) => s.threads).find((t) => t.id === l.thread_id);
    return thread ? [thread] : [];
  }) ?? [];
}

/** The most recently updated thread, if any. */
export function latestThread<T extends { updated_at: string }>(threads: T[]): T | undefined {
  return threads.reduce<T | undefined>((a, b) => !a || b.updated_at >= a.updated_at ? b : a, undefined);
}
