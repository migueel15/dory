import { useEffect } from "react";
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
};

// useLiveUpdates subscribes to the audit SSE stream and invalidates the
// affected queries, batching bursts so a replay triggers one refetch.
export function useLiveUpdates(enabled: boolean) {
  const qc = useQueryClient();
  useEffect(() => {
    if (!enabled) return;
    const pending = new Set<string>();
    let timer: ReturnType<typeof setTimeout> | undefined;
    const source = new EventSource("/api/events/stream?after=latest", { withCredentials: true });
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
  }, [enabled, qc]);
}
