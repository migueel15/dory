import type { Item } from "../api/client";

const POLL_MS = 3000;

// pendingPoll refetches an item query every few seconds while any title is still being generated.
export const pendingPoll = (query: { state: { data?: Item | Item[] } }) => {
  const data = query.state.data;
  const list = Array.isArray(data) ? data : data ? [data] : [];
  return list.some((i) => i.title_source === "pending") ? POLL_MS : false;
};

// ItemTitle shows the title with its source: dimmed and animated while the AI writes it, tagged «IA» after.
export function ItemTitle({ item, className = "" }: { item: Pick<Item, "title" | "title_source">; className?: string }) {
  const pending = item.title_source === "pending";
  return (
    <span className={`inline-flex min-w-0 flex-wrap items-center gap-x-2 gap-y-0.5 ${className}`}>
      <span className={`break-words ${pending ? "opacity-60" : ""}`}>{item.title}</span>
      {pending && (
        <span role="status" className="inline-flex shrink-0 items-center gap-1 text-xs font-normal text-slate-500 dark:text-muted">
          <span aria-hidden="true" className="size-1.5 rounded-full bg-current motion-safe:animate-pulse" />
          Generando título…
        </span>
      )}
      {item.title_source === "ai" && (
        <span title="Título generado por IA" className="shrink-0 rounded bg-violet-100 px-1.5 py-0.5 text-[10px] font-semibold text-violet-700 dark:bg-violet-950/50 dark:text-violet-300">IA</span>
      )}
    </span>
  );
}
