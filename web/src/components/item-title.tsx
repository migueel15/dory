import { useMutation, useQueryClient } from "@tanstack/react-query";
import { api, type Item } from "../api/client";

const POLL_MS = 3000;

// pendingPoll refetches an item query every few seconds while any title is still being generated.
export const pendingPoll = (query: { state: { data?: Item | Item[] } }) => {
  const data = query.state.data;
  const list = Array.isArray(data) ? data : data ? [data] : [];
  return list.some((i) => i.title_source === "pending") ? POLL_MS : false;
};

// ItemTitle shows the title with its source: dimmed and animated while the AI writes it, tagged «IA» after.
// It is inline text, so the tag follows the last word and drops to the next line only when that one is full.
export function ItemTitle({ item, className = "" }: { item: Pick<Item, "title" | "title_source"> & { id?: string }; className?: string }) {
  const pending = item.title_source === "pending";
  const qc = useQueryClient();
  const regenerate = useMutation({
    mutationFn: () => api.regenerateTitle(item.id as string),
    onSettled: () => qc.invalidateQueries({ queryKey: ["items"] }),
  });
  return (
    <span className={`min-w-0 break-words ${className}`}>
      <span className={pending ? "opacity-60" : ""}>{item.title}</span>
      {pending && (
        <span role="status" className="ml-2 inline-flex items-center gap-1 whitespace-nowrap align-middle text-xs font-normal text-slate-500 dark:text-muted">
          <span aria-hidden="true" className="size-1.5 rounded-full bg-current motion-safe:animate-pulse" />
          Generando título…
        </span>
      )}
      {item.title_source === "fallback" && item.id && (
        <button
          type="button"
          title="Regenerar título con IA"
          aria-label="Regenerar título con IA"
          disabled={regenerate.isPending}
          onClick={(e) => { e.stopPropagation(); e.preventDefault(); regenerate.mutate(); }}
          className="ml-1.5 inline-flex size-5 items-center justify-center rounded align-middle text-sm leading-none text-slate-500 hover:bg-slate-200 disabled:opacity-50 dark:text-muted dark:hover:bg-slate-800"
        >
          <span aria-hidden="true" className={regenerate.isPending ? "motion-safe:animate-spin" : ""}>↻</span>
        </button>
      )}
      {item.title_source === "ai" && (
        <span title="Título generado por IA" className="ml-1.5 inline-block whitespace-nowrap rounded bg-violet-100 align-[1px] leading-none px-1.5 py-0.5 text-[10px] font-semibold text-violet-700 dark:bg-violet-950/50 dark:text-violet-300">IA</span>
      )}
    </span>
  );
}
