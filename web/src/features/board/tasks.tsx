import { Plus } from "@phosphor-icons/react";
import { useQueryClient } from "@tanstack/react-query";
import { useNavigate,useSearch } from "@tanstack/react-router";
import { useEffect,useRef,useState } from "react";
import { COLUMN_LABELS,type Card,type Column,type Stage } from "../../api/client";
import { useGithubWatch } from "../../api/live";
import { useBoard } from "../../api/queries/hooks";
import { qk } from "../../api/queries/keys";
import { useMoveTask } from "../../api/queries/tasks";
import { Button,ErrorText } from "../../components/ui";
import { windowCloseInset } from "../../lib/window-close";
import { useProject } from "../../project";
import { IdeaModal } from "../composer/capture";
import { lastActivity } from "./card-layout";
import { ColumnCards,TaskCard } from "./task-card";
import { TaskDetail } from "./task-detail";
import { TaskQuickMenu,type QuickLaunch } from "./task-quick-menu";

// Only these columns can be chosen by hand; the rest come from threads and pull requests.
const MANUAL: Partial<Record<Column, Stage>> = { inbox: "inbox", ready: "ready", cancelled: "cancelled" };
// COLUMNS draws the empty board before the first answer arrives.
const COLUMNS: Column[] = ["inbox", "ready", "working", "needs_you", "in_review", "ready_to_merge", "done", "cancelled"];
// MAIN are the columns drawn as the board; done and cancelled are summarised in the footer.
const MAIN: Column[] = ["inbox", "ready", "working", "needs_you", "in_review", "ready_to_merge"];
const ARCHIVED: Column[] = ["done", "cancelled"];
const DOT: Record<Column, string> = {
  inbox: "bg-slate-400", ready: "bg-indigo-500", working: "bg-blue-500", needs_you: "bg-red-500",
  in_review: "bg-amber-500", ready_to_merge: "bg-emerald-500", done: "bg-emerald-500", cancelled: "bg-slate-400",
};
// Tasks is the kanban, the main view (requirement RF-28).
export function Tasks() {
  const project = useProject();
  const qc = useQueryClient();
  const navigate = useNavigate();
  const search = useSearch({ strict: false }) as { task?: string; launch?: QuickLaunch };
  useGithubWatch();
  const board = useBoard(project, { // Faster while a pull request has checks running, so CI progress shows up soon.
    refetchInterval: (q) => (q.state.data?.cards.some((c) => c.pull && ((c.pull.ci_pending ?? 0) > 0 || c.pull.ci === "PENDING")) ? 15_000 : 30_000) });
  const [adding, setAdding] = useState(false);
  const [drag, setDrag] = useState<string>();
  const [quick, setQuick] = useState<{ c: Card; x: number; y: number }>();
  const [quickError, setQuickError] = useState<unknown>();
  // expanded is the column showing all its cards with its own scroll.
  const [expanded, setExpanded] = useState<Column>();
  const [panel, setPanel] = useState<Column>();
  const footer = useRef<HTMLDivElement>(null);
  const move = useMoveTask({
    onSettled: () => qc.invalidateQueries({ queryKey: qk.board() }),
  });
  // Newest activity first, in every column.
  const cards = [...(board.data?.cards ?? [])].sort((x, y) => lastActivity(y).localeCompare(lastActivity(x)));
  const open = (id?: string, launch?: QuickLaunch) => { setPanel(undefined); navigate({ to: "/tasks", search: (id ? { task: id, ...(launch && { launch }) } : {}) as never }); };
  const cardEl = (c: Card) => <TaskCard key={c.id} c={c} onOpen={() => open(c.id)} onDrag={() => { setQuick(undefined); setDrag(c.id); }} onMenu={(x, y) => { setQuickError(undefined); setQuick({ c, x, y }); }} />;
  useEffect(() => {
    if (!panel) return;
    const key = (e: KeyboardEvent) => { if (e.key === "Escape" && !search.task) setPanel(undefined); };
    const down = (e: MouseEvent) => { if (!footer.current?.contains(e.target as Node)) setPanel(undefined); };
    window.addEventListener("keydown", key);
    window.addEventListener("mousedown", down);
    return () => { window.removeEventListener("keydown", key); window.removeEventListener("mousedown", down); };
  }, [panel, search.task]);
  const drop = (col: Column) => { if (drag && MANUAL[col]) move.mutate({ id: drag, stage: MANUAL[col]! }); setDrag(undefined); };
  return (
    <div className="flex flex-col gap-3 sm:h-[calc(100dvh-3rem-2px)]">
      <div className={`flex shrink-0 items-center gap-2 ${windowCloseInset}`}>
        <h2 className="font-semibold">Kanban</h2>
        <Button className="ml-auto" onClick={() => setAdding(true)}><Plus weight="regular" className="size-4" />Nueva idea</Button>
      </div>
      <ErrorText error={board.error ?? move.error ?? quickError} />
      <div className="flex min-h-0 flex-1 flex-col overflow-hidden rounded-xl border border-slate-200 bg-slate-100 dark:border-border-strong/60 dark:bg-canvas">
        <div className="grid min-h-0 flex-1 grid-cols-1 sm:grid-cols-6 sm:overflow-x-auto sm:[&>section]:min-w-44">
          {MAIN.map((col, i) => {
            const list = cards.filter((c) => c.column === col);
            const all = expanded === col;
            return (
              <section key={col} className={`flex min-h-24 min-w-0 flex-col border-slate-200 dark:border-border max-sm:border-b sm:min-h-0 ${i > 0 ? "sm:border-l" : ""} ${drag && MANUAL[col] ? "bg-blue-50/60 dark:bg-accent-subtle/40" : ""}`}
                onDragOver={(e) => { if (MANUAL[col]) e.preventDefault(); }}
                onDrop={() => drop(col)}>
                <header className="flex h-10 shrink-0 items-center gap-2 border-b border-slate-200 px-3 dark:border-border">
                  <span className={`size-2 rounded-full ${DOT[col]}`} />
                  <h3 className="min-w-0 truncate text-sm font-medium">{COLUMN_LABELS[col]}</h3>
                  {all && <button type="button" onClick={() => setExpanded(undefined)} className="rounded px-1.5 text-xs text-slate-500 hover:bg-slate-100 dark:text-muted dark:hover:bg-raised">Mostrar menos</button>}
                  <span className="ml-auto text-xs tabular-nums text-slate-500 dark:text-muted">{list.length}</span>
                </header>
                <ColumnCards key={project} list={list} all={all} onExpand={() => setExpanded(col)} card={cardEl} />
              </section>
            );
          })}
        </div>
        <div ref={footer} className="relative flex shrink-0 items-center gap-2 border-t border-slate-200 px-3 py-2 dark:border-border">
          {ARCHIVED.map((col) => {
            const n = cards.filter((c) => c.column === col).length;
            return (
              <button key={col} type="button" aria-expanded={panel === col} onClick={() => setPanel(panel === col ? undefined : col)}
                onDragOver={(e) => { if (MANUAL[col]) e.preventDefault(); }} onDrop={() => drop(col)}
                className={`inline-flex items-center gap-1.5 rounded-full border px-3 py-1 text-xs font-medium ${panel === col ? "border-slate-400 bg-slate-100 dark:border-border-strong dark:bg-raised" : "border-slate-200 hover:bg-slate-50 dark:border-border dark:hover:bg-raised"} ${drag && MANUAL[col] ? "border-blue-400 bg-blue-50 dark:bg-accent-subtle" : ""}`}>
                <span className={`size-2 rounded-full ${DOT[col]}`} />{COLUMN_LABELS[col]} · <span className="tabular-nums">{n}</span>
              </button>
            );
          })}
          {panel && (
            <div role="region" aria-label={COLUMN_LABELS[panel]} className="notif-in absolute inset-x-0 bottom-full z-20 border-t border-slate-200 bg-slate-100 shadow-[0_-8px_24px_-12px_rgba(0,0,0,0.3)] dark:border-border-strong/60 dark:bg-canvas">
              <div className="flex items-start gap-2 overflow-x-auto p-2" onWheel={(e) => { if (e.deltaX === 0) e.currentTarget.scrollLeft += e.deltaY; }}>
                {cards.filter((c) => c.column === panel).map((c) => (
                  <div key={c.id} className="w-60 shrink-0 [&>div]:cursor-pointer">{cardEl(c)}</div>
                ))}
                {!cards.some((c) => c.column === panel) && <p className="px-2 py-3 text-sm text-slate-500 dark:text-muted">Sin tareas.</p>}
              </div>
            </div>
          )}
        </div>
      </div>
      {adding && <IdeaModal onClose={() => setAdding(false)} projectId={project || undefined} />}
      {quick && <TaskQuickMenu key={quick.c.id} {...quick} onClose={() => setQuick(undefined)} onOpen={(launch) => open(quick.c.id, launch)} onError={setQuickError} />}
      {search.task && <TaskDetail key={search.task} id={search.task} launch={search.launch} onClose={() => open()} />}
    </div>
  );
}
