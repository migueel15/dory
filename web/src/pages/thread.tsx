import { useCallback, useEffect, useRef, useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { useParams } from "@tanstack/react-router";
import { ArrowUp, SidebarSimple, Paperclip } from "@phosphor-icons/react";
import { api, type T3ThreadView } from "../api/client";
import { useThreadLive } from "../api/live";
import { ErrorText, pill } from "../components/ui";
import { FullscreenHost } from "./github-diff";
import { ContextBar, ThreadDetails } from "./thread/header";
import { Chat, Insert } from "./thread/chat";
import { Changes, OpenTurnContext, TurnTarget } from "./thread/changes";

// The thread page is split in ./thread/: header, chat, composer, queue and changes.
export type { Insert } from "./thread/chat";

// ThreadView shows one T3 Code thread: conversation and composer on the left,
// changed files on the right.
// useThreadView loads a thread's view live and tracks the run the user just
// stopped: the view shows it stopped until T3 confirms, and goes back if T3
// keeps it running.
function useThreadView(threadId: string) {
  const key = ["t3-view", threadId];
  // Safety net for a stalled live stream: while a turn runs, reload every 15 s.
  // Subagents can keep working after the parent's turn ends: poll while any does.
  const view = useQuery({ queryKey: key, queryFn: () => api.t3View(threadId), refetchInterval: (q) =>
    q.state.data?.subagents?.some((a) => ["running", "starting", "preparing", "waiting", "pending"].includes(a.status)) ? 4_000 : q.state.data?.active_run_id ? 15_000 : false });
  // The agent edits the worktree during a turn: refresh its diff with each thread event.
  useThreadLive(threadId, key, [["t3-diff", threadId]]);
  const [stopping, setStopping] = useState("");
  const running = view.data?.active_run_id ?? "";
  useEffect(() => {
    if (!stopping) return;
    if (running !== stopping) { setStopping(""); return; }
    const t = setTimeout(() => setStopping(""), 15_000);
    return () => clearTimeout(t);
  }, [stopping, running]);
  const v: T3ThreadView | undefined = view.data && stopping && view.data.active_run_id === stopping
    ? { ...view.data, active_run_id: "", active_status: "", interrupted: true }
    : view.data;
  return { view, v, key, setStopping };
}

// ThreadChat is the thread's chat (messages, questions, queue and composer),
// shared by the thread page and other views that embed a thread. notice shows
// above the composer; beforeSend runs before each message and, if it fails,
// the message is not sent.
// insert adds text to the composer each time its seq changes.
export function ThreadChat({ threadId, notice, beforeSend, insert }: { threadId: string; notice?: React.ReactNode; beforeSend?: () => Promise<unknown>; insert?: Insert }) {
  const { view, v, key, setStopping } = useThreadView(threadId);
  if (view.isPending) return <ChatLoading />;
  // A failed refresh keeps the conversation on screen (and its scroll, drafts
  // and open dialogs); only a first load that fails shows the error.
  if (!v) return <ErrorText error={view.error} />;
  return <Chat key={v.id} v={v} queryKey={key} onStop={setStopping} notice={notice} beforeSend={beforeSend} insert={insert} />;
}

export function ThreadView() {
  const { threadId } = useParams({ from: "/threads/$threadId" });
  const { view, v, key, setStopping } = useThreadView(threadId);
  const [panel, setPanel] = useState<"" | "changes">("");
  // The turn (and file) a changes card asked the panel to show.
  const [target, setTarget] = useState<TurnTarget & { seq: number }>();
  const openTurn = useCallback((t: TurnTarget) => { setTarget((p) => ({ ...t, seq: (p?.seq ?? 0) + 1 })); setPanel("changes"); }, []);
  const aside = useRef<HTMLElement>(null);
  const [host, setHost] = useState<HTMLDivElement | null>(null);
  const [opened, setOpened] = useState(false);
  useEffect(() => { if (panel) setOpened(true); }, [panel]);
  const [width, setWidth] = useState(() => { try { return Number(localStorage.getItem("dory.thread-panel-width")) || 560; } catch { return 560; } });
  if (view.isPending) return <div className="flex h-full min-h-0 flex-col"><ChatLoading /></div>;
  // A failed refresh keeps the conversation on screen (and its scroll, drafts
  // and open dialogs); only a first load that fails shows the error.
  if (!v) return <ErrorText error={view.error} />;
  return (
    // The panel's divider runs the full height: the title belongs to the chat
    // column. A diff in full screen fills both columns, not the thread list or
    // the app sidebar.
    <div ref={setHost} className="relative flex h-full min-h-0">
      <FullscreenHost.Provider value={host}>
      {/* With the panel open, the chat column keeps the same gap to the divider
          as the panel does (lg:pl-1), so the two header buttons sit symmetric. */}
      <div className={`min-h-0 min-w-0 flex-1 flex-col gap-3 ${panel ? "hidden lg:flex lg:pr-1" : "flex"}`}>
      <ContextBar v={v}>
        <ThreadDetails v={v} onChanges={() => setPanel("changes")} panelOpen={panel !== ""} />
        {/* With the panel open, its toggle moves to the panel's own top row. */}
        {!panel && (
          <button type="button" aria-pressed={false} onClick={() => setPanel("changes")} title="Panel de cambios" aria-label="Panel de cambios" className={`${pill} !px-2`}>
            <SidebarSimple className="size-4 -scale-x-100" />
          </button>
        )}
      </ContextBar>
        <div className="flex min-h-0 flex-1 flex-col"><OpenTurnContext.Provider value={openTurn}><Chat key={v.id} v={v} queryKey={key} onStop={setStopping} /></OpenTurnContext.Provider></div>
      </div>
        {/* Always mounted (its content only once opened): on desktop its width
            slides between 0 and the panel width, with the content pinned right
            so it enters from the right edge and leaves the same way. */}
        {(
          <aside ref={aside} aria-hidden={!panel}
            className={`min-h-0 flex-col overflow-hidden lg:flex lg:shrink-0 lg:transition-[width] lg:duration-300 lg:ease-out ${panel ? "flex w-full lg:w-[var(--panel-w)]" : "hidden lg:w-0"}`}
            style={{ "--panel-w": `${width}px` } as React.CSSProperties}>
            <div className="ml-auto flex min-h-0 w-full flex-1 lg:w-[var(--panel-w)]">
              <ResizeHandle panel={aside} width={width} onResize={setWidth} />
              <div className="flex min-h-0 min-w-0 flex-1 flex-col pb-4 lg:pl-1">{opened && <Changes v={v} queryKey={key} target={target} onClose={() => setPanel("")} />}</div>
            </div>
          </aside>
        )}
      </FullscreenHost.Provider>
    </div>
  );
}

// ChatLoading keeps the chat's frame while the conversation loads: a notice in
// the middle and the composer's shape at the bottom, so nothing jumps after.
function ChatLoading() {
  return (
    <section aria-busy="true" className="relative flex min-h-0 flex-1 flex-col">
      <div className="grid min-h-0 flex-1 place-items-center">
        <p className="flex items-center gap-2 text-sm text-slate-500 motion-safe:animate-pulse dark:text-muted">Cargando conversación…</p>
      </div>
      <div className="mx-auto w-full max-w-3xl px-3 pb-4 pt-2 sm:px-6">
        <div className="rounded-3xl border border-slate-200 bg-white shadow-md dark:border-border-strong dark:bg-surface">
          <textarea disabled rows={1} aria-label="Mensaje" placeholder="Escribe al hilo…" className="block w-full resize-none bg-transparent px-4 pb-1 pt-3.5 text-[15px] leading-6 placeholder:text-slate-400" />
          <div className="flex items-center gap-0.5 px-2 pb-2">
            <span className="grid size-8 place-items-center text-slate-300 dark:text-muted"><Paperclip className="size-4" /></span>
            <span className="ml-auto grid size-8 place-items-center rounded-full bg-slate-200 text-white dark:bg-raised"><ArrowUp weight="bold" className="size-4" /></span>
          </div>
        </div>
      </div>
    </section>
  );
}

// ResizeHandle drags the side panel's left edge. While dragging it only moves
// a CSS variable on the panel, once per frame, so React does not re-render the
// chat and the diff on every pointer move; the width is saved on release.
export function ResizeHandle({ panel, width, onResize, storageKey = "dory.thread-panel-width", min = 320, rest = 420 }: { panel: React.RefObject<HTMLElement | null>; width: number; onResize: (w: number) => void; storageKey?: string | null; min?: number; rest?: number }) {
  const start = (e: React.PointerEvent) => {
    e.preventDefault();
    const x0 = e.clientX, w0 = width;
    let last = w0, frame = 0;
    const move = (ev: PointerEvent) => {
      last = Math.min(Math.max(min, w0 + x0 - ev.clientX), window.innerWidth - rest);
      if (!frame) frame = requestAnimationFrame(() => { frame = 0; panel.current?.style.setProperty("--panel-w", `${last}px`); });
    };
    const up = () => {
      cancelAnimationFrame(frame);
      panel.current?.style.removeProperty("transition");
      onResize(last);
      window.removeEventListener("pointermove", move); window.removeEventListener("pointerup", up);
      document.body.style.cursor = ""; document.body.style.userSelect = "";
      if (storageKey) try { localStorage.setItem(storageKey, String(Math.round(last))); } catch { /* per-browser only */ }
    };
    document.body.style.cursor = "col-resize"; document.body.style.userSelect = "none";
    panel.current?.style.setProperty("transition", "none");
    window.addEventListener("pointermove", move); window.addEventListener("pointerup", up);
  };
  return (
    <div role="separator" aria-orientation="vertical" aria-label="Redimensionar panel" onPointerDown={start}
      className="group hidden w-2 shrink-0 cursor-col-resize items-stretch justify-center lg:flex">
      <span className="w-px bg-slate-200 transition-colors group-hover:w-0.5 group-hover:bg-slate-400 dark:bg-border dark:group-hover:bg-muted" />
    </div>
  );
}
