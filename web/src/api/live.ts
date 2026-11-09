import { useEffect, useState } from "react";

// useVisible tracks whether this tab is on screen. Live streams only stay open
// while it is: over plain HTTP a browser allows about six connections per host,
// and two per background tab would leave a new tab's streams waiting.
export function useVisible() {
  const [visible, setVisible] = useState(() => typeof document === "undefined" || document.visibilityState !== "hidden");
  useEffect(() => {
    const on = () => setVisible(document.visibilityState !== "hidden");
    document.addEventListener("visibilitychange", on);
    return () => document.removeEventListener("visibilitychange", on);
  }, []);
  return visible;
}
import { useQueryClient } from "@tanstack/react-query";

// Query keys refreshed for each audit entity_type.
const keys: Record<string, string[]> = {
  item: ["items"],
  feature: ["features"],
  project: ["projects"],
  repository: ["repositories"],
  repositorylocation: ["repositories"],
  environment: ["environments"],
  memory: ["memories"],
  link: ["items", "memories"],
  itemthread: ["item-threads", "refinement"],
  itemplan: ["refinement", "items"],
  // Signals from the T3 monitor: thread states changed in some instance.
  t3: ["t3-status", "item-threads", "issue-threads", "refinement", "t3-context"],
};

// useLiveUpdates subscribes to the audit SSE stream and invalidates the
// affected queries, batching bursts so a replay triggers one refetch.
export function useLiveUpdates(enabled: boolean) {
  const qc = useQueryClient();
  const visible = useVisible();
  useEffect(() => {
    if (!enabled || !visible) return;
    const pending = new Set<string>();
    let timer: ReturnType<typeof setTimeout> | undefined;
    const source = new EventSource("/api/events/stream?after=latest", { withCredentials: true });
    // Back from the background (or a reconnect): what changed meanwhile was not streamed.
    let opened = false;
    source.addEventListener("open", () => { if (opened) qc.invalidateQueries(); opened = true; });
    source.addEventListener("change", (e) => {
      const { entity_type } = JSON.parse((e as MessageEvent).data) as { entity_type: string };
      for (const k of keys[entity_type] ?? []) pending.add(k);
      clearTimeout(timer);
      timer = setTimeout(() => {
        for (const k of pending) qc.invalidateQueries({ queryKey: [k] });
        pending.clear();
      }, 300);
    });
    return () => { clearTimeout(timer); source.close(); };
  }, [enabled, visible, qc]);
  // Returning to the tab reloads what changed while its stream was closed.
  const [seen, setSeen] = useState(visible);
  useEffect(() => { if (visible && !seen) qc.invalidateQueries(); setSeen(visible); }, [visible]); // eslint-disable-line react-hooks/exhaustive-deps
}

// useThreadLive follows one T3 thread while a view shows its conversation and
// refetches the given query on each burst of changes. EventSource reconnects
// on its own if the server or T3 drops the stream.
export function useThreadLive(threadId: string | undefined, queryKey: unknown[], alsoKeys: unknown[][] = []) {
  const qc = useQueryClient();
  const key = JSON.stringify([queryKey, ...alsoKeys]);
  const visible = useVisible();
  useEffect(() => {
    if (!threadId || !visible) return;
    const source = new EventSource(`/api/t3/threads/${encodeURIComponent(threadId)}/stream`, { withCredentials: true });
    const refresh = () => { for (const k of JSON.parse(key) as unknown[][]) qc.invalidateQueries({ queryKey: k }); };
    source.addEventListener("thread", refresh);
    // The server skips the first T3 snapshot of each connection, so whatever changed
    // while the stream was down (T3 dropped it, the browser reconnected) is lost: reload on every (re)open.
    source.addEventListener("open", refresh);
    return () => source.close();
  }, [threadId, key, visible, qc]);
}
