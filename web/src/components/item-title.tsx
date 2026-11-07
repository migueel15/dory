import type { Item } from "../api/client";

const POLL_MS = 3000;

// pendingPoll refetches an item query every few seconds while any title is still being generated.
export const pendingPoll = (query: { state: { data?: Item | Item[] } }) => {
  const data = query.state.data;
  const list = Array.isArray(data) ? data : data ? [data] : [];
  return list.some((i) => i.title_source === "pending") ? POLL_MS : false;
};

// ItemTitle shows the title with its source: dimmed and animated while the AI writes it, tagged «IA» after.
// It is inline text, so the tag follows the last word and drops to the next line only when that one is full.
export function ItemTitle({ item, className = "" }: { item: Pick<Item, "title" | "title_source">; className?: string }) {
  const pending = item.title_source === "pending";
  return (
    <span className={`min-w-0 break-words ${className}`}>
      <span className={pending ? "opacity-60" : ""}>{item.title}</span>
      {pending && (
        <span role="status" className="ml-2 inline-flex items-center gap-1 whitespace-nowrap align-middle text-xs font-normal text-slate-500 dark:text-muted">
          <span aria-hidden="true" className="size-1.5 rounded-full bg-current motion-safe:animate-pulse" />
          Generando título…
        </span>
      )}
      {item.title_source === "ai" && (
        <span title="Título generado por IA" className="ml-1.5 inline-block whitespace-nowrap rounded bg-violet-100 align-[1px] leading-none px-1.5 py-0.5 text-[10px] font-semibold text-violet-700 dark:bg-violet-950/50 dark:text-violet-300">IA</span>
      )}
    </span>
  );
}
