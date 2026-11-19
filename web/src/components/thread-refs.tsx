import { useEffect, useRef, useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { CheckCircle, CircleDashed, GitMerge, GitPullRequest, RadioButton, XCircle, type Icon } from "@phosphor-icons/react";
import { api, type Card } from "../api/client";
import { HoverCard } from "./tooltip";

// A pull request or issue tied to a thread through its task, with its state.
export type GhRef = { kind: "pull" | "issue"; number: number; title: string; state: string; repo?: string };

const LOOK: Record<string, { icon: Icon; tone: string; label: string }> = {
  "pull:open": { icon: GitPullRequest, tone: "text-green-600 dark:text-green-400", label: "Abierta" },
  "pull:draft": { icon: GitPullRequest, tone: "text-slate-400", label: "Borrador" },
  "pull:merged": { icon: GitMerge, tone: "text-purple-600 dark:text-purple-400", label: "Mergeada" },
  "pull:closed": { icon: GitPullRequest, tone: "text-red-600 dark:text-red-400", label: "Cerrada" },
  "issue:open": { icon: RadioButton, tone: "text-green-600 dark:text-green-400", label: "Abierta" },
  "issue:completed": { icon: CheckCircle, tone: "text-purple-600 dark:text-purple-400", label: "Completada" },
  "issue:not_planned": { icon: XCircle, tone: "text-slate-400", label: "No planificada" },
  "issue:closed": { icon: CheckCircle, tone: "text-purple-600 dark:text-purple-400", label: "Cerrada" },
};
const look = (r: GhRef) => LOOK[`${r.kind}:${r.state}`] ?? { icon: CircleDashed, tone: "text-slate-400", label: "Desconocido" };

// useThreadRefs maps each task to its pull request and issue, with the state
// from the board and titles from the open GitHub items (closed ones may lack it).
export function useThreadRefs() {
  const board = useQuery({ queryKey: ["board", ""], queryFn: () => api.board(), refetchInterval: 30_000 });
  const gh = useQuery({ queryKey: ["github"], queryFn: () => api.github(), staleTime: 60_000, retry: false });
  const byTask = new Map<string, GhRef[]>();
  for (const c of board.data?.cards ?? []) {
    const refs: GhRef[] = [];
    if (c.pr_number) refs.push(pullRef(c));
    if (c.issue_number) {
      const open = gh.data?.issues.find((i) => i.number === c.issue_number && (!c.repo || i.repo === c.repo));
      refs.push({
        kind: "issue",
        number: c.issue_number,
        repo: c.repo,
        title: open?.title ?? "",
        state: open ? "open" : gh.data ? "closed" : "",
      });
    }
    if (refs.length) byTask.set(c.id, refs);
  }
  return (taskId: string | null) => (taskId ? byTask.get(taskId) ?? [] : []);
}

function pullRef(c: Card): GhRef {
  return {
    kind: "pull",
    number: c.pr_number!,
    repo: c.pull?.repo || c.repo,
    title: c.pull?.title ?? "",
    state: c.pr_state ?? (c.pull ? (c.pull.draft ? "draft" : "open") : ""),
  };
}

// RefChip shows the reference and reveals its title on hover.
export function RefChip({ r, listVisible }: { r: GhRef; listVisible: boolean }) {
  const [anchor, setAnchor] = useState<HTMLElement | null>(null);
  const chip = useRef<HTMLSpanElement>(null);
  const [inView, setInView] = useState(false);
  const { icon: Glyph, tone, label } = look(r);
  const isPull = r.kind === "pull";
  const labelKind = isPull ? "PR" : "Issue";
  const kind = isPull ? "pulls" : "issues";
  const repo = r.repo ?? "";
  const needsTitle = !r.title && !!repo;
  useEffect(() => {
    if (!listVisible || !needsTitle || !chip.current) return;
    const observer = new IntersectionObserver(([entry]) => setInView(entry.isIntersecting));
    observer.observe(chip.current);
    return () => observer.disconnect();
  }, [listVisible, needsTitle]);

  const detail = useQuery({
    queryKey: ["github-detail", repo, kind, r.number],
    queryFn: () => api.githubDetail(repo, kind, r.number),
    enabled: needsTitle && (Boolean(anchor) || (listVisible && inView)),
    staleTime: 60_000,
    retry: false,
  });
  const title = r.title || (isPull ? detail.data?.pull?.title : detail.data?.issue?.title);
  return (
    <span ref={chip} onPointerEnter={(e) => setAnchor(e.currentTarget)} onPointerLeave={() => setAnchor(null)} aria-label={`${labelKind} #${r.number} · ${label}`}
      className={`inline-flex shrink-0 items-center gap-0.5 text-[0.6875rem] tabular-nums ${tone}`}>
      <Glyph className="size-3.5" />#{r.number}
      {anchor && (
        <HoverCard key={title ?? ""} anchor={anchor}>
          {/* One line inside the tooltip bubble: state icon, number and title. */}
          <div className="flex max-w-80 items-center gap-2">
            <Glyph className={`size-3.5 shrink-0 ${tone}`} />
            <span className="shrink-0 font-semibold tabular-nums">#{r.number}</span>
            {title && <span className="min-w-0 truncate">{title}</span>}
          </div>
        </HoverCard>
      )}
    </span>
  );
}
