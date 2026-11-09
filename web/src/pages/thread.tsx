import { useEffect, useLayoutEffect, useRef, useState } from "react";
import { createPortal } from "react-dom";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Link, useParams } from "@tanstack/react-router";
import { ArrowDown, ArrowRight, ArrowSquareOut, ArrowUp, ArrowsOut, Browser, Cube, LinkSimple, Eye, EyeSlash, Key, Cloud, FolderSimple, GitBranch, GitPullRequest, Info, Kanban, Brain, CaretDown, CaretRight, Check, Copy, FileText, ListChecks, GitDiff, SidebarSimple, Paperclip, Lightning, ListBullets, PencilSimple, Play, Question, Quotes, Stop, Trash, Wrench, X } from "@phosphor-icons/react";
import { api, type T3Attachment, type T3QueuedRun, type T3ThreadView, type T3ViewMessage } from "../api/client";
import { useThreadLive } from "../api/live";
import { Button, copyText, ErrorText, Input, Md, errMsg, uuid, Notice, Select, StatusPill, pill } from "../components/ui";
import { OpenInT3 } from "../components/thread-link";
import { PendingRequest, type QuestionControl } from "../components/pending-request";
import { SelectionMenu } from "../components/selection-menu";
import { RefLink, ThreadMediaContext } from "../components/refs";
import { ProviderIcon } from "./launch-form";
import { saveEffort, savedEffort } from "../effort";
import { Diff, type Suggestion } from "./github-diff";
import { PdfThumb } from "../components/pdf-thumb";
import { useComposerSuggest } from "../components/composer-suggest";
import { paintTokens, QUOTE_RE, quoteBlock, quoteLabel, tokenRanges, TokenChip, type Token, type Tokens } from "../components/composer-tokens";
import { defaultUrlTransform } from "react-markdown";

const runtimeModes = [
  { value: "approval-required", label: "Pedir permiso" },
  { value: "auto-accept-edits", label: "Aceptar ediciones" },
  { value: "auto", label: "Automático" },
  { value: "full-access", label: "Acceso total" },
];

// ThreadView shows one T3 Code thread: conversation and composer on the left,
// changed files on the right.
// useThreadView loads a thread's view live and tracks the run the user just
// stopped: the view shows it stopped until T3 confirms, and goes back if T3
// keeps it running.
function useThreadView(threadId: string) {
  const key = ["t3-view", threadId];
  // Safety net for a stalled live stream: while a turn runs, reload every 15 s.
  const view = useQuery({ queryKey: key, queryFn: () => api.t3View(threadId), refetchInterval: (q) => q.state.data?.active_run_id ? 15_000 : false });
  useThreadLive(threadId, key);
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
  if (view.error || !v) return <ErrorText error={view.error} />;
  return <Chat key={v.id} v={v} queryKey={key} onStop={setStopping} notice={notice} beforeSend={beforeSend} insert={insert} />;
}

export function ThreadView() {
  const { threadId } = useParams({ from: "/threads/$threadId" });
  const { view, v, key, setStopping } = useThreadView(threadId);
  const [panel, setPanel] = useState<"" | "changes">("");
  const aside = useRef<HTMLElement>(null);
  const [opened, setOpened] = useState(false);
  useEffect(() => { if (panel) setOpened(true); }, [panel]);
  const [width, setWidth] = useState(() => { try { return Number(localStorage.getItem("dory.thread-panel-width")) || 560; } catch { return 560; } });
  if (view.isPending) return <div className="flex h-full min-h-0 flex-col"><ChatLoading /></div>;
  if (view.error || !v) return <ErrorText error={view.error} />;
  return (
    <div className="flex h-full min-h-0 flex-col gap-3">
      <ContextBar v={v}>
        <ThreadDetails v={v} />
        <button type="button" aria-pressed={panel === "changes"} onClick={() => setPanel(panel === "changes" ? "" : "changes")} title="Panel de cambios" aria-label="Panel de cambios"
          className={`${pill} !px-2 ${panel === "changes" ? "!border-slate-500 !text-slate-900 dark:!text-white" : ""}`}>
          <SidebarSimple className="size-4 -scale-x-100" />
        </button>
      </ContextBar>
      <div className="relative flex min-h-0 flex-1">
        <div className={`min-h-0 min-w-0 flex-1 flex-col ${panel ? "hidden lg:flex" : "flex"}`}><Chat key={v.id} v={v} queryKey={key} onStop={setStopping} /></div>
        {/* Always mounted (its content only once opened): on desktop its width
            slides between 0 and the panel width, with the content pinned right
            so it enters from the right edge and leaves the same way. */}
        {(
          <aside ref={aside} aria-hidden={!panel}
            className={`min-h-0 flex-col overflow-hidden lg:flex lg:shrink-0 lg:transition-[width] lg:duration-300 lg:ease-out ${panel ? "flex w-full lg:w-[var(--panel-w)]" : "hidden lg:w-0"}`}
            style={{ "--panel-w": `${width}px` } as React.CSSProperties}>
            <div className="ml-auto flex min-h-0 w-full flex-1 lg:w-[var(--panel-w)]">
              <ResizeHandle panel={aside} width={width} onResize={setWidth} />
              <div className="flex min-h-0 min-w-0 flex-1 flex-col pb-4 lg:pl-3">{opened && <Changes v={v} queryKey={key} />}</div>
            </div>
          </aside>
        )}
      </div>
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

function ContextBar({ v, children }: { v: T3ThreadView; children?: React.ReactNode }) {
  const t3 = useQuery({ queryKey: ["t3-status"], queryFn: api.t3Status, refetchInterval: 60_000 });
  const t = t3.data?.flatMap((x) => x.threads).find((x) => x.id === v.id);
  return (
    <header className="relative flex flex-wrap items-center gap-2">
      <h1 className="min-w-0 truncate text-lg font-semibold">{v.title || v.id}</h1>
      {t?.url && <OpenInT3 url={t.url} environmentId={t.t3_environment_id} threadId={v.id} className="mr-1" />}
      {v.pending && <StatusPill tone="amber">Espera tu respuesta</StatusPill>}
      {children}
    </header>
  );
}

// repoName turns a repository URL or key into owner/name, lowercase.
const repoName = (url = "") => url.replace(/\.git$/, "").split("/").slice(-2).join("/").toLowerCase();

// diffStats counts files and added/removed lines in a unified diff.
const diffStats = (diff = "") => {
  let files = 0, add = 0, del = 0;
  for (const l of diff.split("\n")) {
    if (l.startsWith("diff --git")) files++;
    else if (l.startsWith("+") && !l.startsWith("+++")) add++;
    else if (l.startsWith("-") && !l.startsWith("---")) del++;
  }
  return { files, add, del };
};

// ThreadDetails is the header button that drops a card down-left with the
// thread's references, styled like T3 Code's: environment, branch, pull
// requests, items and changes.
function ThreadDetails({ v }: { v: T3ThreadView }) {
  const [open, setOpen] = useState(false);
  const box = useRef<HTMLDivElement>(null);
  // Everything loads in the background as the thread opens, so the card is ready
  // when opened; the chat never waits for it.
  const t3 = useQuery({ queryKey: ["t3-status"], queryFn: api.t3Status, refetchInterval: 60_000 });
  const envs = useQuery({ queryKey: ["environments"], queryFn: () => api.environments.list(), staleTime: 5 * 60_000 });
  const first = v.pull_requests.map((p) => prOf(p.url)).find(Boolean);
  // Same keys as the PR page and the changes panel, so the fetches are shared.
  const pull = useQuery({ queryKey: ["github-detail", first?.repo, "pulls", first?.number], queryFn: () => api.githubDetail(first!.repo, "pulls", first!.number), enabled: !!first, staleTime: 30_000 });
  const diff = useQuery({ queryKey: ["t3-diff", v.id, v.messages.length], queryFn: () => api.t3Diff(v.id), staleTime: 30_000 });
  const envId = t3.data?.find((x) => x.instance_id === v.instance_id)?.environment_id;
  const catalog = useQuery({ queryKey: ["t3-catalog", v.instance_id], queryFn: () => api.t3Catalog(v.instance_id), enabled: !!v.instance_id, staleTime: 60_000 });
  const project = catalog.data?.projects.find((p) => p.id === v.project_id);
  const env = envs.data?.find((e) => e.id === envId);
  const pr = pull.data?.pull;
  // T3 only knows the branch of threads it runs in a worktree; else the PR's head branch fills in.
  // Dory's own git diff of the worktree wins: it includes uncommitted work. Else the PR's numbers.
  const git = diff.data?.source === "git" ? diff.data : undefined;
  const branch = v.branch || git?.branch || pr?.head_ref || "";
  const worktree = v.worktree_path || git?.worktree || "";
  const local = diffStats(diff.data?.diff);
  const changes = git || !pr ? { add: local.add, del: local.del, base: git?.base ?? "", pending: diff.isPending } : { add: pr.additions, del: pr.deletions, base: pr.base_ref, pending: pull.isPending };
  useEffect(() => {
    if (!open) return;
    const away = (e: MouseEvent) => !box.current?.contains(e.target as Node) && setOpen(false);
    const esc = (e: KeyboardEvent) => e.key === "Escape" && setOpen(false);
    document.addEventListener("mousedown", away);
    window.addEventListener("keydown", esc);
    return () => { document.removeEventListener("mousedown", away); window.removeEventListener("keydown", esc); };
  }, [open]);
  const ico = "size-[18px] shrink-0 text-slate-500 dark:text-muted";
  const rowCls = "flex min-h-9 items-center gap-3 rounded-md px-2 text-sm";
  return (
    <div ref={box} className="ml-auto">
      <button type="button" aria-expanded={open} aria-haspopup="dialog" onClick={() => setOpen(!open)} title="Detalles del hilo" aria-label="Detalles del hilo"
        className={`${pill} !px-2 ${open ? "!border-slate-500 !text-slate-900 dark:!text-white" : ""}`}>
        <Info className="size-4" />
      </button>
      {open && (
        <div role="dialog" aria-label="Detalles del hilo" className="notif-in absolute right-0 top-full z-50 mt-1.5 w-80 max-w-[calc(100vw-2rem)] origin-top-right divide-y divide-slate-200 rounded-xl border border-slate-200 bg-white p-1.5 shadow-xl dark:divide-border dark:border-border dark:bg-surface">
          <div className="pb-1.5">
            <div className={rowCls}>
              <Cloud className={ico} />
              <span className="min-w-0 flex-1 truncate">{env?.name ?? (envId ? envId.slice(0, 8) : "Entorno desconocido")}</span>
              {worktree && <CopyText text={worktree} hidden icon={<FolderSimple className="size-[18px]" />} title={`Copiar ruta: ${worktree}`} />}
            </div>
          </div>
          <div className="py-1.5">
            <div className={rowCls} title={project?.root}>
              <Cube className={ico} />
              {project ? <span className="min-w-0 flex-1 truncate"><span className="font-medium">{project.title}</span>{project.repo && <span className="text-slate-500 dark:text-muted"> · {repoName(project.repo)}</span>}</span>
                : <span className="flex-1 text-slate-400 dark:text-muted">{catalog.isPending ? "…" : "Sin proyecto"}</span>}
            </div>
            <div className={rowCls}>
              <GitBranch className={ico} />
              {branch ? <span className="min-w-0 flex-1 truncate font-medium" title={branch}>{branch}</span> : <span className="flex-1 text-slate-400 dark:text-muted">Sin rama</span>}
              {branch && <CopyText text={branch} hidden title="Copiar rama" />}
            </div>
            {v.pull_requests.map((p, i) => <PullRow key={p.url} link={p} pull={i === 0 ? pr ?? undefined : undefined} onOpen={() => setOpen(false)} />)}
            {v.items.map((it) => (
              <Link key={it.item_id} to="/items/$id/define" params={{ id: it.item_id }} onClick={() => setOpen(false)} className={`${rowCls} hover:bg-slate-100 dark:hover:bg-raised`}>
                <Kanban className={ico} />
                <span className="flex-1">Item <span className="font-mono text-xs text-slate-500 dark:text-muted">{it.item_id.slice(0, 8)}</span></span>
                <CaretRight weight="regular" className="size-3.5 text-slate-400" />
              </Link>
            ))}
          </div>
          <div className="pt-1.5">
            <div className={rowCls}>
              <GitDiff className={ico} />
              <span className="min-w-0 flex-1 truncate"><span className="font-medium">Cambios</span>{changes.base && <span className="text-slate-500 dark:text-muted"> vs {changes.base}</span>}</span>
              {changes.pending ? <span className="text-xs text-slate-400">…</span> : (
                <span className="font-mono text-xs tabular-nums"><span className="text-emerald-600 dark:text-emerald-400">+{changes.add}</span> <span className="text-red-600 dark:text-red-400">−{changes.del}</span></span>
              )}
            </div>
          </div>
        </div>
      )}
    </div>
  );
}

const prBadge = (p?: { draft: boolean; state: string }) => {
  if (!p) return null;
  const s = p.state.toLowerCase();
  const [label, cls] = s === "merged" ? ["Merged", "bg-violet-600"] : s === "closed" ? ["Closed", "bg-red-600"] : p.draft ? ["Draft", "bg-slate-500"] : ["Ready", "bg-blue-600"];
  return <span className={`rounded-md px-2 py-0.5 text-xs font-semibold text-white ${cls}`}>{label}</span>;
};

// PullRow is a pull request in the details card: title, number and state;
// it opens the PR inside Dory, and the arrow opens it on GitHub.
function PullRow({ link, pull, onOpen }: { link: { url: string; number: number; repository: string }; pull?: { title: string; draft: boolean; state: string }; onOpen: () => void }) {
  const p = prOf(link.url);
  const [owner, repo] = (p?.repo ?? link.repository).split("/");
  const body = (
    <>
      <GitPullRequest className="mt-0.5 size-[18px] shrink-0 text-slate-500 dark:text-muted" />
      <span className="min-w-0 flex-1">
        <span className="line-clamp-2 font-medium">{pull?.title ?? link.repository}</span>
        <span className="mt-1 flex items-center gap-2">
          <span className="font-mono text-xs text-slate-500 dark:text-muted">#{link.number}</span>
          <span className="ml-auto">{prBadge(pull)}</span>
        </span>
      </span>
    </>
  );
  const cls = "flex min-w-0 flex-1 items-start gap-3 rounded-md px-2 py-1.5 text-sm hover:bg-slate-100 dark:hover:bg-raised";
  return (
    <div className="flex items-start gap-0.5">
      {p ? <Link to="/github/$owner/$repo/$kind/$number" params={{ owner, repo, kind: "pulls", number: String(p.number) }} onClick={onOpen} className={cls} title="Abrir la PR en Dory">{body}</Link>
        : <a href={link.url} target="_blank" rel="noreferrer" className={cls}>{body}</a>}
      <a href={link.url} target="_blank" rel="noreferrer" title="Abrir en GitHub" aria-label="Abrir en GitHub"
        className="mt-1 grid size-7 shrink-0 place-items-center rounded-md text-slate-400 hover:bg-slate-100 hover:text-slate-900 dark:hover:bg-raised dark:hover:text-white"><ArrowSquareOut className="size-4" /></a>
    </div>
  );
}

// CopyText shows a value with a copy button; hidden shows only the button.
function CopyText({ text, mono, small, hidden, icon, title = "Copiar" }: { text: string; mono?: boolean; small?: boolean; hidden?: boolean; icon?: React.ReactNode; title?: string }) {
  const [done, setDone] = useState(false);
  const copy = () => void copyText(text).then((ok) => { if (ok) { setDone(true); setTimeout(() => setDone(false), 1200); } });
  return (
    <span className="flex min-w-0 items-center gap-1.5">
      {!hidden && <span className={`min-w-0 truncate ${mono ? "font-mono" : ""} ${small ? "text-xs" : "text-[13px]"}`} title={text}>{text}</span>}
      <button type="button" onClick={copy} title={title} aria-label={title} className="shrink-0 text-slate-400 hover:text-slate-900 dark:hover:text-white">
        {done ? <Check weight="regular" className="size-3.5" /> : icon ?? <Copy className="size-3.5" />}
      </button>
    </span>
  );
}

// Insert adds text to the composer; with quote, a quote object goes before it.
export type Insert = { text: string; seq: number; quote?: { label: string; body: string } };
function Chat({ v, queryKey, onStop, notice, beforeSend, insert }: { v: T3ThreadView; queryKey: unknown[]; onStop: (run: string) => void; notice?: React.ReactNode; beforeSend?: () => Promise<unknown>; insert?: Insert }) {
  const qc = useQueryClient();
  const end = useRef<HTMLDivElement>(null);
  const scroller = useRef<HTMLDivElement>(null);
  // Text sent to the composer: from the host view (insert) or from a selection.
  const [ins, setIns] = useState<Insert>();
  const push = (text: string, quote?: Insert["quote"]) => setIns((p) => ({ text, quote, seq: (p?.seq ?? 0) + 1 }));
  useEffect(() => { if (insert) push(insert.text, insert.quote); }, [insert?.seq]); // eslint-disable-line react-hooks/exhaustive-deps
  // While a question is open, text sent from the composer answers it.
  const [question, setQuestion] = useState<QuestionControl | null>(null);
  const onControl = (c: QuestionControl | null) => setQuestion((p) => (p && c && p.picked === c.picked && p.last === c.last && p.busy === c.busy ? (p.next = c.next, p) : c));
  const [away, setAway] = useState(false);
  // T3 sends only the latest turns: older pages load when the top comes near,
  // keeping what is on screen on the same pixel.
  const [older, setOlder] = useState<T3ViewMessage[]>([]);
  const [cursor, setCursor] = useState<string | null>(null);
  const [loadingOlder, setLoadingOlder] = useState(false);
  const anchor = useRef<number | null>(null);
  const nextCursor = cursor ?? v.history_cursor;
  const loadOlder = () => {
    if (!nextCursor || loadingOlder) return;
    setLoadingOlder(true);
    anchor.current = scroller.current ? scroller.current.scrollHeight - scroller.current.scrollTop : null;
    api.t3History(v.id, nextCursor).then(
      (p) => { setOlder((o) => [...p.messages, ...o]); setCursor(p.has_more ? p.next_cursor : ""); },
      () => { anchor.current = null; },
    ).finally(() => setLoadingOlder(false));
  };
  useLayoutEffect(() => {
    const el = scroller.current;
    if (el && anchor.current !== null) { el.scrollTop = el.scrollHeight - anchor.current; anchor.current = null; }
  }, [older]);
  const onScroll = () => {
    const el = scroller.current;
    if (!el) return;
    setAway(el.scrollHeight - el.scrollTop - el.clientHeight > 200);
    if (el.scrollTop < 300) loadOlder();
  };
  const recent = new Set(v.messages.map((m) => m.id));
  // Only messages that arrive after opening the thread animate in; the loaded history stays still.
  const initial = useRef<Set<string> | null>(null);
  if (!initial.current) initial.current = new Set(recent);
  const seen = new Set<string>();
  // Pages overlap the latest turns: the live copy of an entry wins.
  const all = [...older.filter((m) => !recent.has(m.id) && !seen.has(m.id) && seen.add(m.id)), ...v.messages];
  const last = v.messages.at(-1);
  useEffect(() => { end.current?.scrollIntoView({ block: "end" }); }, [v.messages.length, last?.text.length, !!v.pending, v.queue.length]);
  const pendingSecret = [...all].reverse().find((m) => m.role === "secret" && m.status === "pending");
  const working = !!v.active_run_id && !last?.streaming && (last?.role === "user" || last?.role === "assistant" || !last);
  return (
    <ThreadMediaContext.Provider value={v.id}>
    <section className="relative flex min-h-0 flex-1 flex-col">
      <SelectionMenu box={scroller} scope="[data-message]" label="Usar la selección en el mensaje" actions={[
        { label: "Preguntar", icon: <Question className="size-3.5" />, run: (t) => push("Pregunta: ", { label: quoteLabel(t), body: t }) },
        { label: "Citar", icon: <Quotes className="size-3.5" />, run: (t) => push("", { label: quoteLabel(t), body: t }) },
      ]} />
      <div ref={scroller} onScroll={onScroll} className="min-h-0 flex-1 overflow-y-auto py-3"><div className="mx-auto w-full max-w-3xl space-y-4 px-3 sm:px-6">
        {v.messages.length === 0 && <p className="py-10 text-center text-sm text-slate-500 dark:text-muted">Sin mensajes todavía. Escribe abajo para empezar.</p>}
        {nextCursor && (
          <div className="flex justify-center">
            <button type="button" onClick={loadOlder} disabled={loadingOlder} className="inline-flex items-center gap-1.5 rounded-full px-3 py-1 text-xs text-slate-500 hover:bg-slate-100 hover:text-slate-900 dark:text-muted dark:hover:bg-raised dark:hover:text-white">
              {loadingOlder ? <span className="size-3 animate-spin rounded-full border-2 border-current border-t-transparent" /> : <ArrowUp weight="regular" className="size-3.5" />}
              {loadingOlder ? "Cargando mensajes anteriores…" : "Cargar mensajes anteriores"}
            </button>
          </div>
        )}
        {groups(all).map((m, gi, all) => Array.isArray(m) ? <WorkBlock key={m[0].id} steps={m} live={!!v.active_run_id && gi === all.length - 1} /> : m.role === "plan" ? <PlanCard key={m.id} m={m} /> : m.role === "handoff" ? <Handoff key={m.id} m={m} /> : m.role === "secret" ? <SecretLine key={m.id} m={m} /> : m.role === "html" && m.html ? <HtmlRender key={m.id} threadId={v.id} page={m.html} /> : m.role === "interrupt" ? (
          <div key={m.id} role="separator" className="flex items-center gap-3 text-[11px] font-medium uppercase tracking-wide text-slate-400">
            <span className="h-px flex-1 bg-slate-200 dark:bg-border" /><Stop weight="fill" className="size-3" />{m.text}<span className="h-px flex-1 bg-slate-200 dark:bg-border" />
          </div>
        ) : m.role === "user" ? (
          <div key={m.id} data-message className={`group flex flex-col items-end ${initial.current?.has(m.id) ? "" : "msg-in"}`}>
            {!!m.attachments?.length && <SentAttachments threadId={v.id} list={m.attachments} />}
            {(() => {
              // Tokens sent from the composer show inline as objects (tok: links the Md renders).
              const shown = tokenize(m);
              return <>
                {shown && <div className="max-w-[85%] rounded-2xl rounded-br-md bg-slate-900 px-4 py-2.5 text-white shadow-sm dark:bg-accent [&_*]:!text-inherit [&_code]:!bg-white/15 [&_pre]:!border-white/15 [&_pre]:!bg-white/10 [&_pre_code]:!bg-transparent"><Md urlTransform={keepToken} components={tokenLinks}>{shown}</Md></div>}
              </>;
            })()}
            <MessageMeta m={m} />
          </div>
        ) : (
          <div key={m.id} data-message className={m.role === "system" ? "text-xs text-slate-500 dark:text-muted" : "group max-w-full pr-6"}>
            {m.text ? <Md>{m.text}</Md> : null}
            {m.streaming ? <Dots /> : m.role === "assistant" && <MessageMeta m={m} />}
          </div>
        ))}
        {working && <Dots />}
        <Queue v={v} queryKey={queryKey} />
        <div ref={end} />
      </div></div>
      <div className="relative h-0">
        {/* Always mounted so it can slide up on show and down on hide. */}
        {(
          <button type="button" onClick={() => end.current?.scrollIntoView({ block: "end", behavior: "smooth" })} aria-label="Ir al final" aria-hidden={!away} tabIndex={away ? 0 : -1}
            className={`absolute bottom-3 left-1/2 z-20 inline-flex -translate-x-1/2 items-center gap-1.5 transition-[translate,opacity] duration-200 ease-out ${away ? "translate-y-0 opacity-100" : "pointer-events-none translate-y-4 opacity-0"} rounded-full border border-slate-200 bg-white px-3 py-1.5 text-xs font-medium text-slate-600 shadow-md hover:bg-slate-50 hover:text-slate-900 dark:border-border-strong dark:bg-surface dark:text-text-secondary dark:hover:bg-raised dark:hover:text-white`}>
            <ArrowDown weight="bold" className="size-3.5" />Ir al final
          </button>
        )}
      </div>
      <div className="mx-auto w-full max-w-3xl px-3 pb-4 pt-2 sm:px-6">
        {!v.pending && pendingSecret && (
          // A secret request slides out like a question, with its own input.
          <div className="notif-in mx-3 -mb-6 rounded-t-3xl border border-b-0 border-slate-200 bg-white px-4 pb-9 pt-3 shadow-md dark:border-border-strong dark:bg-surface">
            <p className="mb-3 flex items-center gap-1.5 text-xs font-medium text-slate-500 dark:text-muted"><Key className="size-4" />El agente pide un secreto</p>
            <SecretRequest key={pendingSecret.id} threadId={v.id} m={pendingSecret} onDone={() => qc.invalidateQueries({ queryKey })} />
          </div>
        )}
        {v.pending && (
          // The question slides out from behind the composer, sharing its look.
          <div className="notif-in mx-3 -mb-6 max-h-[55dvh] overflow-y-auto rounded-t-3xl border border-b-0 border-slate-200 bg-white px-4 pb-9 pt-3 shadow-md dark:border-border-strong dark:bg-surface">
            <p className="mb-3 flex items-center gap-1.5 text-xs font-medium text-slate-500 dark:text-muted"><Question className="size-4" />{v.pending.kind === "user_input" ? "El agente te pregunta" : "El agente pide permiso"}</p>
            <PendingRequest key={v.pending.id} threadId={v.id} pending={v.pending} onDone={() => qc.invalidateQueries({ queryKey })} onControl={onControl} />
          </div>
        )}
        {notice}
        <div className="relative z-10"><Composer v={v} queryKey={queryKey} onStop={onStop} question={question} beforeSend={beforeSend} insert={ins} /></div>
      </div>
    </section>
    </ThreadMediaContext.Provider>
  );
}

const fullDate = new Intl.DateTimeFormat("es-ES", { weekday: "long", day: "numeric", month: "long", year: "numeric", hour: "2-digit", minute: "2-digit", second: "2-digit" });
const shortTime = new Intl.DateTimeFormat("es-ES", { hour: "2-digit", minute: "2-digit" });

// MessageMeta shows, on hover, when a message was sent and a button to copy it.
function MessageMeta({ m }: { m: T3ViewMessage }) {
  const [copied, setCopied] = useState(false);
  const at = m.created_at ? new Date(m.created_at) : undefined;
  const valid = at && !Number.isNaN(at.getTime());
  const copy = async () => {
    if (await copyText(m.text)) { setCopied(true); setTimeout(() => setCopied(false), 1500); }
  };
  return (
    <div className="mt-1 flex items-center gap-1 text-[11px] text-slate-400 transition-opacity sm:opacity-0 sm:group-hover:opacity-100 sm:focus-within:opacity-100">
      {valid && <time dateTime={m.created_at} title={fullDate.format(at)} className="cursor-default tabular-nums">{shortTime.format(at)}</time>}
      <button type="button" onClick={copy} title={copied ? "Copiado" : "Copiar mensaje"} aria-label="Copiar mensaje" className={iconBtn}>
        {copied ? <Check weight="regular" className="size-3.5" /> : <Copy className="size-3.5" />}
      </button>
    </div>
  );
}

// PlanCard shows a plan the model proposed as a readable markdown document,
// folded to a preview when long.
function PlanCard({ m }: { m: T3ViewMessage }) {
  const [open, setOpen] = useState(false);
  const card = useRef<HTMLElement>(null);
  const bottom = useRef<number | null>(null);
  // Folding keeps what follows the card on the same pixel: the chat scrolls
  // by exactly what the card shrank.
  useLayoutEffect(() => {
    const before = bottom.current;
    bottom.current = null;
    const el = card.current;
    if (before === null || !el) return;
    let box: HTMLElement | null = el.parentElement;
    while (box && !/(auto|scroll)/.test(getComputedStyle(box).overflowY)) box = box.parentElement;
    if (box) box.scrollTop += el.getBoundingClientRect().bottom - before;
  }, [open]);
  const toggle = () => { bottom.current = open ? card.current?.getBoundingClientRect().bottom ?? null : null; setOpen(!open); };
  const [copied, setCopied] = useState(false);
  const long = m.text.length > 1200;
  const title = m.text.match(/^#\s+(.+)$/m)?.[1];
  const copy = async () => { if (await copyText(m.text)) { setCopied(true); setTimeout(() => setCopied(false), 1500); } };
  return (
    <article ref={card} className="group overflow-hidden rounded-2xl border border-slate-200 bg-white shadow-sm dark:border-border dark:bg-surface">
      <header className="flex items-center gap-2 border-b border-slate-200 bg-slate-50 px-4 py-2.5 dark:border-border dark:bg-raised">
        <ListChecks className="size-4 shrink-0 text-slate-500 dark:text-muted" />
        <span className="text-xs font-semibold uppercase tracking-wide text-slate-500 dark:text-muted">Plan propuesto</span>
        {m.streaming && <span className="size-3 animate-spin rounded-full border-2 border-current border-t-transparent text-slate-400" />}
        <button type="button" onClick={copy} title={copied ? "Copiado" : "Copiar plan en markdown"} aria-label="Copiar plan" className={`ml-auto ${iconBtn}`}>
          {copied ? <Check weight="regular" className="size-3.5" /> : <Copy className="size-3.5" />}
        </button>
      </header>
      <div className={`relative px-5 py-4 ${long && !open ? "max-h-96 overflow-hidden" : ""}`}>
        <Md>{m.text || "…"}</Md>
        {long && !open && <div className="pointer-events-none absolute inset-x-0 bottom-0 h-24 bg-gradient-to-t from-white dark:from-surface" />}
      </div>
      {long && (
        <button type="button" onClick={toggle} className="flex w-full items-center justify-center gap-1 border-t border-slate-200 py-2 text-xs font-medium text-slate-500 hover:bg-slate-50 hover:text-slate-900 dark:border-border dark:text-muted dark:hover:bg-raised dark:hover:text-white">
          {open ? <>Contraer <CaretDown weight="regular" className="size-3 rotate-180" /></> : <>Ver plan completo{title ? ` · ${title}` : ""} <CaretDown weight="regular" className="size-3" /></>}
        </button>
      )}
    </article>
  );
}

const isStep = (m: T3ViewMessage) => m.role === "reasoning" || m.role === "tool";

// groups folds each run of consecutive reasoning and tool steps into one block.
function groups(ms: T3ViewMessage[]): (T3ViewMessage | T3ViewMessage[])[] {
  const out: (T3ViewMessage | T3ViewMessage[])[] = [];
  for (const m of ms) {
    const prev = out.at(-1);
    if (!isStep(m)) out.push(m);
    else if (Array.isArray(prev)) prev.push(m);
    else out.push([m]);
  }
  return out;
}

const stepRunning = (m: T3ViewMessage) => m.streaming || (m.role === "tool" && !["completed", "failed", "error", "declined", "interrupted", "cancelled"].includes(m.status ?? ""));
const stepFailed = (m: T3ViewMessage) => m.status === "failed" || m.status === "error";
const stepLabel = (m: T3ViewMessage) => m.role === "reasoning" ? (m.streaming ? "Pensando…" : "Razonamiento") : m.title || "Herramienta";

function StepIcon({ m }: { m: T3ViewMessage }) {
  if (stepFailed(m)) return <X weight="regular" className="size-3.5 shrink-0 text-red-500" />;
  if (stepRunning(m)) return <span className="size-3 shrink-0 animate-spin rounded-full border-2 border-current border-t-transparent" />;
  return m.role === "reasoning" ? <Brain className="size-3.5 shrink-0" /> : <Wrench className="size-3.5 shrink-0" />;
}

// WorkBlock is one label that follows the model's current step; unfolded it
// lists every thought and tool of that stretch, each foldable on its own.
function WorkBlock({ steps, live }: { steps: T3ViewMessage[]; live: boolean }) {
  const [open, setOpen] = useState(false);
  const last = steps[steps.length - 1];
  const running = live || steps.some(stepRunning);
  const tools = steps.filter((m) => m.role === "tool").length;
  const summary = running ? stepLabel(last) : `${steps.length} pasos${tools ? ` · ${tools} herramientas` : ""}`;
  const preview = running && (last.role === "tool" ? last.detail : last.text.split("\n").filter(Boolean).at(-1));
  return (
    <div className="text-sm">
      <button type="button" aria-expanded={open} onClick={() => setOpen(!open)}
        className="flex max-w-full items-center gap-2 rounded-lg py-0.5 text-xs text-slate-500 hover:text-slate-900 dark:text-muted dark:hover:text-white">
        {running ? <StepIcon m={{ ...last, streaming: true }} /> : <Check weight="regular" className="size-3.5 shrink-0" />}
        <span className="shrink-0 font-medium">{summary}</span>
        {preview && <span className="min-w-0 truncate font-mono opacity-70">{preview}</span>}
        {open ? <CaretDown weight="regular" className="size-3 shrink-0" /> : <CaretRight weight="regular" className="size-3 shrink-0" />}
      </button>
      {open && (
        <div className="mt-1 space-y-1 border-l-2 border-slate-200 pl-3 dark:border-border">
          {steps.map((m) => <Step key={m.id} m={m} />)}
        </div>
      )}
    </div>
  );
}

// Step is one thought or tool call inside a WorkBlock; it unfolds its text or input.
function Step({ m }: { m: T3ViewMessage }) {
  const body = m.role === "reasoning" ? m.text : m.detail ?? "";
  const [open, setOpen] = useState<boolean | null>(null);
  const shown = open ?? (m.role === "reasoning" && m.streaming);
  return (
    <div className="text-xs text-slate-500 dark:text-muted">
      <button type="button" disabled={!body} onClick={() => setOpen(!shown)} className="flex max-w-full items-center gap-2 py-0.5 hover:text-slate-900 disabled:hover:text-inherit dark:hover:text-white">
        <StepIcon m={m} />
        <span className="shrink-0 font-medium text-slate-700 dark:text-text-secondary">{stepLabel(m)}</span>
        {!shown && m.role === "tool" && body && <code className="min-w-0 truncate font-mono">{body}</code>}
        {body && (shown ? <CaretDown weight="regular" className="size-3 shrink-0" /> : <CaretRight weight="regular" className="size-3 shrink-0" />)}
      </button>
      {shown && body && (m.role === "reasoning"
        ? <div className="pb-1 pl-5 text-slate-500 dark:text-muted [&_*]:!text-inherit"><Md>{body}</Md></div>
        : <pre className="mb-1 ml-5 max-h-60 overflow-auto whitespace-pre-wrap break-all rounded bg-slate-100 p-2 font-mono text-[11px] dark:bg-raised">{body}</pre>)}
    </div>
  );
}

const Dots = () => (
  <span className="inline-flex items-center gap-1 py-1" aria-label="Trabajando">
    {[0, 150, 300].map((d) => <span key={d} className="size-1.5 rounded-full bg-slate-400 motion-safe:animate-bounce" style={{ animationDelay: `${d}ms` }} />)}
  </span>
);

// Queue shows the messages waiting for the active turn as outlined bubbles
// at the end of the conversation, each with its actions underneath.
function Queue({ v, queryKey }: { v: T3ThreadView; queryKey: unknown[] }) {
  const qc = useQueryClient();
  const done = () => qc.invalidateQueries({ queryKey });
  const act = useMutation({ mutationFn: (fn: () => Promise<void>) => fn(), onSuccess: done });
  if (!v.queue.length) return null;
  const q = v.queue;
  return (
    <section aria-label="Mensajes en cola" className="space-y-3 border-t border-dashed border-slate-300 pt-3 dark:border-border">
      {q.map((r) => (
        <QueuedRow key={r.run_id} r={r} canSteer={!!v.active_run_id}
          onSteer={() => act.mutate(() => api.t3QueueSteer(v.id, r.run_id))}
          onCancel={() => act.mutate(() => api.t3QueueCancel(v.id, r.run_id))}
          onEdit={(text) => act.mutate(() => api.t3QueueEdit(v.id, r.run_id, text))} />
      ))}
      <ErrorText error={act.error} />
    </section>
  );
}

const iconBtn = "grid size-7 place-items-center rounded-full text-slate-400 hover:bg-slate-100 hover:text-slate-900 disabled:opacity-30 dark:hover:bg-raised dark:hover:text-white";

function QueuedRow({ r, canSteer, onSteer, onCancel, onEdit }: { r: T3QueuedRun; canSteer: boolean; onSteer: () => void; onCancel: () => void; onEdit: (text: string) => void }) {
  const [draft, setDraft] = useState<string | null>(null);
  return (
    <div className="msg-in flex flex-col items-end gap-1">
      <div className="w-fit max-w-[85%] rounded-2xl rounded-br-md border border-dashed border-slate-400 px-4 py-2.5 text-slate-700 dark:border-border-strong dark:text-text-secondary">
        {draft === null ? <p className="whitespace-pre-wrap break-words text-sm">{r.text}</p> : (
          <form className="flex gap-1" onSubmit={(e) => { e.preventDefault(); onEdit(draft); setDraft(null); }}>
            <Input autoFocus value={draft} onChange={(e) => setDraft(e.target.value)} onKeyDown={(e) => { if (e.key === "Escape") setDraft(null); }} />
            <Button>Guardar</Button>
          </form>
        )}
      </div>
      <div className="flex items-center gap-0.5">
        <button type="button" className={iconBtn} title="Mandar ya (steer: inyectar en el turno en curso)" disabled={!canSteer} onClick={onSteer}><Lightning /></button>
        <button type="button" className={iconBtn} title="Editar" onClick={() => setDraft(r.text)}><PencilSimple /></button>
        <button type="button" className={iconBtn} title="Eliminar de la cola" onClick={onCancel}><Trash /></button>
      </div>
    </div>
  );
}

type MenuOption = { value: string; label: string; group?: string; icon?: React.ReactNode };

// Chip is a compact dropdown for the composer toolbar; the menu opens upwards.
function Chip({ value, options, onChange, title, icon }: { value: string; options: MenuOption[]; onChange: (v: string) => void; title: string; icon?: React.ReactNode }) {
  const [open, setOpen] = useState(false);
  const box = useRef<HTMLDivElement>(null);
  useEffect(() => {
    if (!open) return;
    const close = (e: PointerEvent) => { if (!box.current?.contains(e.target as Node)) setOpen(false); };
    document.addEventListener("pointerdown", close);
    return () => document.removeEventListener("pointerdown", close);
  }, [open]);
  const current = options.find((o) => o.value === value);
  return (
    <div ref={box} className="relative">
      <button type="button" title={title} aria-haspopup="listbox" aria-expanded={open} onClick={() => setOpen(!open)}
        onKeyDown={(e) => { if (e.key === "Escape") setOpen(false); }}
        className="inline-flex h-8 max-w-[11rem] items-center gap-1.5 rounded-full px-2.5 text-xs font-medium text-slate-600 transition-colors hover:bg-slate-100 hover:text-slate-900 dark:text-text-secondary dark:hover:bg-raised dark:hover:text-white">
        {current?.icon ?? icon}<span className="truncate">{current?.label ?? value}</span><CaretDown weight="regular" className="size-3 shrink-0 opacity-60" />
      </button>
      {open && (
        <ul role="listbox" className="absolute bottom-full left-0 z-20 mb-2 max-h-72 min-w-[12rem] overflow-y-auto rounded-xl border border-slate-200 bg-white p-1 shadow-lg dark:border-border dark:bg-surface">
          {options.map((o, i) => (
            <li key={o.value}>
              {o.group && o.group !== options[i - 1]?.group && <p className="px-2 pb-1 pt-2 text-[11px] font-semibold uppercase tracking-wide text-slate-400">{o.group}</p>}
              <button type="button" role="option" aria-selected={o.value === value} onClick={() => { onChange(o.value); setOpen(false); }}
                className="flex w-full items-center gap-2 rounded-lg px-2 py-1.5 text-left text-sm hover:bg-slate-100 dark:hover:bg-raised">
                {o.icon}<span className="min-w-0 flex-1 truncate">{o.label}</span>{o.value === value && <Check weight="regular" className="size-3.5" />}
              </button>
            </li>
          ))}
        </ul>
      )}
    </div>
  );
}

// ModelPicker joins model and effort in one chip; its panel lists models on
// the left and, on the right, the efforts of the model last clicked. Picking a
// model preselects the effort last used with it.
function ModelPicker({ models, selected, effort, effortsOf, onChange }: { models: MenuOption[]; selected: string; effort: string; effortsOf: (value: string) => { values: string[]; initial: string } | undefined; onChange: (value: string, effort: string) => void }) {
  const [open, setOpen] = useState(false);
  const [focus, setFocus] = useState(selected);
  const box = useRef<HTMLDivElement>(null);
  useEffect(() => {
    if (!open) return;
    setFocus(selected);
    const close = (e: PointerEvent) => { if (!box.current?.contains(e.target as Node)) setOpen(false); };
    document.addEventListener("pointerdown", close);
    return () => document.removeEventListener("pointerdown", close);
  }, [open, selected]);
  const current = models.find((m) => m.value === selected);
  const efforts = effortsOf(focus);
  const pickModel = (value: string) => {
    const e = effortsOf(value);
    setFocus(value);
    onChange(value, e?.initial ?? "");
    if (!e) setOpen(false);
  };
  const row = "flex w-full items-center gap-2 rounded-lg px-2 py-1.5 text-left text-sm hover:bg-slate-100 dark:hover:bg-raised";
  return (
    <div ref={box} className="relative">
      <button type="button" title="Modelo y esfuerzo" aria-haspopup="dialog" aria-expanded={open} onClick={() => setOpen(!open)}
        onKeyDown={(e) => { if (e.key === "Escape") setOpen(false); }}
        className="inline-flex h-8 max-w-[15rem] items-center gap-1.5 rounded-full px-2.5 text-xs font-medium text-slate-600 transition-colors hover:bg-slate-100 hover:text-slate-900 dark:text-text-secondary dark:hover:bg-raised dark:hover:text-white">
        {current?.icon}<span className="truncate">{current?.label ?? selected}</span>
        {effort && <span className="shrink-0 text-slate-400">· {effort}</span>}<CaretDown weight="regular" className="size-3 shrink-0 opacity-60" />
      </button>
      {open && (
        <div role="dialog" aria-label="Modelo y esfuerzo" className="absolute bottom-full left-0 z-20 mb-2 flex max-h-80 w-[min(30rem,calc(100vw-2rem))] overflow-hidden rounded-xl border border-slate-200 bg-white shadow-lg dark:border-border dark:bg-surface">
          <ul className="min-w-0 flex-1 overflow-y-auto p-1">
            {models.map((o, i) => (
              <li key={o.value}>
                {o.group && o.group !== models[i - 1]?.group && <p className="px-2 pb-1 pt-2 text-[11px] font-semibold uppercase tracking-wide text-slate-400">{o.group}</p>}
                <button type="button" onClick={() => pickModel(o.value)}
                  className={`${row} ${o.value === focus ? "bg-slate-100 dark:bg-raised" : ""}`}>
                  {o.icon}<span className="min-w-0 flex-1 truncate">{o.label}</span>{o.value === selected && <Check weight="regular" className="size-3.5" />}
                </button>
              </li>
            ))}
          </ul>
          <div className="w-32 shrink-0 overflow-y-auto border-l border-slate-200 p-1 dark:border-border">
            <p className="px-2 pb-1 pt-2 text-[11px] font-semibold uppercase tracking-wide text-slate-400">Esfuerzo</p>
            {efforts ? efforts.values.map((x) => {
              const active = focus === selected ? x === effort : x === efforts.initial;
              return (
                <button key={x} type="button" onClick={() => { onChange(focus, x); setOpen(false); }} className={`${row} ${active ? "font-medium" : ""}`}>
                  <span className="min-w-0 flex-1 truncate">{x}</span>{active && <Check weight="regular" className="size-3.5" />}
                </button>
              );
            }) : <p className="px-2 py-1.5 text-xs text-slate-400">Sin opciones</p>}
          </div>
        </div>
      )}
    </div>
  );
}

type Draft = { key: string; name: string; mime: string; preview: string; done?: T3Attachment; error?: string };

function Composer({ v, queryKey, onStop, question, beforeSend, insert }: { v: T3ThreadView; queryKey: unknown[]; onStop: (run: string) => void; question: QuestionControl | null; beforeSend?: () => Promise<unknown>; insert?: Insert }) {
  const qc = useQueryClient();
  const catalog = useQuery({ queryKey: ["t3-catalog", v.instance_id], queryFn: () => api.t3Catalog(v.instance_id), enabled: !!v.instance_id, staleTime: 60_000 });
  const [text, setText] = useState("");
  const [choice, setChoice] = useState("");
  const [effort, setEffort] = useState("");
  const area = useRef<HTMLTextAreaElement>(null);
  const picker = useRef<HTMLInputElement>(null);
  const [files, setFiles] = useState<Draft[]>([]);
  const uploading = files.some((f) => !f.done && !f.error);
  const ready = files.flatMap((f) => (f.done ? [f.done] : []));
  // Each file uploads as soon as it is added; the message carries the results.
  const addFiles = (list: FileList | File[]) => {
    for (const file of Array.from(list)) {
      const key = uuid(); // crypto.randomUUID needs HTTPS, the dev preview is plain HTTP
      const named = file.name ? file : new File([file], `imagen-${new Date().toISOString().slice(11, 19).replace(/:/g, "")}.png`, { type: file.type });
      const preview = URL.createObjectURL(named); // also opens PDFs and text in the preview
      setFiles((fs) => [...fs, { key, name: named.name, mime: named.type, preview }]);
      api.t3Upload(v.id, named).then(
        (done) => setFiles((fs) => fs.map((f) => (f.key === key ? { ...f, done } : f))),
        (e: unknown) => setFiles((fs) => fs.map((f) => (f.key === key ? { ...f, error: errMsg(e) || "No se pudo subir" } : f))),
      );
    }
  };
  const [viewing, setViewing] = useState<Shown | null>(null);
  const removeFile = (key: string) => setFiles((fs) => fs.filter((f) => { if (f.key === key) URL.revokeObjectURL(f.preview); return f.key !== key; }));
  const selected = choice || `${v.provider_id}|${v.model}`;
  const [providerId, slug] = selected.split("|");
  // / commands and $ skills come from the provider that will run the message.
  const prov = catalog.data?.providers.find((p) => p.instance_id === providerId);
  // The repository's own commands and skills (T3 scans the thread folder) load in the background;
  // until then, and if the scan fails, the provider's global ones from the catalog are used.
  const composer = useQuery({ queryKey: ["t3-composer", v.id, providerId], queryFn: () => api.t3Composer(v.id, providerId), enabled: !!providerId, staleTime: 5 * 60_000, retry: false });
  const projectRepo = repoName(catalog.data?.projects.find((p) => p.id === v.project_id)?.repo);
  // Picked /commands, $skills, @files and #PRs live in the text as tokens: the
  // text holds their characters, tokens their reference (a file's full path).
  const [tokens, setTokens] = useState<Tokens>({});
  const addToken = (insert: string, t: Token) => {
    // Two files with the same name: the second one is written with its path.
    const key = t.kind === "path" && tokens[insert] && tokens[insert].value !== t.value ? `@${t.value}` : insert;
    setTokens((ts) => ({ ...ts, [key]: t }));
    return key;
  };
  useEffect(() => {
    if (!insert) return;
    let add = insert.text;
    if (insert.quote) {
      // The quote shows as one object; a repeated label gets a number.
      let key = `«${insert.quote.label}»`;
      for (let n = 2; tokens[key] && tokens[key].value !== insert.quote.body; n++) key = `«${insert.quote.label} (${n})»`;
      setTokens((ts) => ({ ...ts, [key]: { kind: "quote", value: insert.quote!.body, title: insert.quote!.label } }));
      add = `${key} ${insert.text}`;
    }
    setText((t) => (t.trim() ? `${t.trimEnd()}\n${add}` : add));
    setTimeout(() => { const el = area.current; if (el) { el.focus(); el.setSelectionRange(el.value.length, el.value.length); } }, 0);
  }, [insert?.seq]); // eslint-disable-line react-hooks/exhaustive-deps
  const ranges = tokenRanges(text, tokens);
  const present = ranges.map((r) => tokens[r.text]);
  const mentions = [...new Set(present.filter((t) => t.kind === "path").map((t) => t.value))];
  const skillsUsed = [...new Set(present.filter((t) => t.kind === "skill").map((t) => t.value))];
  // T3 gets each @file with its full path; the other tokens go as written.
  // T3 gets each @file with its full path and each quote as a block quote; the other tokens go as written.
  const outgoing = ranges.reduceRight((t, r) => {
    const tk = tokens[r.text];
    if (tk.kind === "path") return `${t.slice(0, r.start)}@${tk.value}${t.slice(r.end)}`;
    if (tk.kind === "quote") return `${t.slice(0, r.start)}${quoteBlock(tk.title ?? "", tk.value)}${t.slice(r.end).replace(/^ /, "")}`;
    return t;
  }, text).trim();
  const paint = useRef<HTMLDivElement>(null);
  // A token deletes as a whole: Backspace at its end or inside, Delete at its start or inside.
  // A selection never splits a token: an edge inside one grows to cover it, and
  // a caret inside one jumps past it in the direction it was moving (arrows
  // step over a token in one press), or to the nearest edge on a click.
  const lastCaret = useRef(0);
  const snapSelection = () => {
    const el = area.current;
    if (!el) return;
    const { selectionStart: s0, selectionEnd: e0, selectionDirection: dir } = el;
    let s = s0, en = e0;
    for (const r of ranges) {
      if (s0 === e0) {
        if (s0 > r.start && s0 < r.end) {
          const prev = lastCaret.current;
          s = en = prev <= r.start ? r.end : prev >= r.end ? r.start : s0 - r.start < r.end - s0 ? r.start : r.end;
        }
      } else {
        if (s > r.start && s < r.end) s = r.start;
        if (en > r.start && en < r.end) en = r.end;
      }
    }
    if (s !== s0 || en !== e0) el.setSelectionRange(s, en, dir ?? undefined);
    lastCaret.current = s === en ? s : dir === "backward" ? s : en;
  };
  const deleteToken = (e: React.KeyboardEvent<HTMLTextAreaElement>) => {
    const el = e.currentTarget;
    if ((e.key !== "Backspace" && e.key !== "Delete") || el.selectionStart !== el.selectionEnd) return false;
    const at = el.selectionStart;
    // Ctrl/Alt/Cmd delete a word: across the spaces up to a token, the token is the word.
    const word = e.ctrlKey || e.altKey || e.metaKey;
    const reach = (r: { start: number; end: number }) => e.key === "Backspace"
      ? (at > r.start && at <= r.end) || (word && at > r.end && !text.slice(r.end, at).trim())
      : (at >= r.start && at < r.end) || (word && at < r.start && !text.slice(at, r.start).trim());
    const hit = ranges.find(reach);
    if (!hit) return false;
    e.preventDefault();
    // A word delete also takes the spaces between the caret and the token.
    const from = e.key === "Backspace" ? hit.start : Math.min(at, hit.start);
    const to = e.key === "Backspace" ? Math.max(at, hit.end) : hit.end;
    setText(text.slice(0, from) + text.slice(to));
    requestAnimationFrame(() => el.setSelectionRange(from, from));
    return true;
  };
  const suggest = useComposerSuggest({ threadId: v.id, text, setText, area, commands: composer.data?.commands ?? prov?.commands ?? [], skills: composer.data?.skills ?? prov?.skills ?? [], repo: projectRepo, onToken: addToken });
  const model = catalog.data?.providers.find((p) => p.instance_id === providerId)?.models.find((m) => m.slug === slug);
  const effortValue = effort || (choice ? (model?.effort ? savedEffort(model.slug, model.effort.values) || model.effort.default : "") : v.effort);
  const changed = !!choice || !!effort;

  // Grow the textarea with its content, up to about ten lines.
  useEffect(() => {
    const el = area.current;
    if (!el) return;
    el.style.height = "auto";
    el.style.height = `${Math.min(el.scrollHeight, 240)}px`;
  }, [text]);

  const send = useMutation({
    // While a turn runs, Ctrl/Cmd+Enter steers and Enter (or the button) queues.
    mutationFn: async (asSteer?: boolean) => { await beforeSend?.(); return api.t3Dispatch(v.id, {
      // Inline @labels go out as @full/path (what the agent reads) and as context records.
      text: outgoing,
      mode: asSteer ? "steer" : "auto", ...(ready.length ? { attachments: ready } : {}), skills: skillsUsed, mentions,
      ...(changed ? { provider_id: providerId, model: slug, effort_option: model?.effort?.id, effort: model?.effort ? effortValue : undefined } : {}),
    }); },
    onSuccess: () => { if (changed && model?.effort && effortValue) saveEffort(model.slug, effortValue); setText(""); setTokens({}); suggest.reset(); files.forEach((f) => URL.revokeObjectURL(f.preview)); setFiles([]); qc.invalidateQueries({ queryKey }); },
  });
  const settings = useMutation({ mutationFn: (b: { interaction_mode?: string; runtime_mode?: string }) => api.t3Settings(v.id, b), onSuccess: () => qc.invalidateQueries({ queryKey }) });
  const busy = !!v.active_run_id;
  const models: MenuOption[] = (catalog.data?.providers ?? []).flatMap((p) => p.models.map((m) => ({ value: `${p.instance_id}|${m.slug}`, label: m.name, group: p.name, icon: <ProviderIcon driver={p.driver || p.name} model={`${m.slug} ${m.name}`} /> })));
  if (!models.some((m) => m.value === selected)) models.unshift({ value: selected, label: v.model || "Modelo del hilo", icon: <ProviderIcon driver={v.provider_id} model={v.model} /> });
  // Optimistic: the thread shows stopped at once; an error brings the run back.
  const stop = useMutation({
    mutationFn: () => api.t3Interrupt(v.id),
    onMutate: () => onStop(v.active_run_id),
    onError: () => onStop(""),
    onSettled: () => qc.invalidateQueries({ queryKey }),
  });
  const held = !busy && v.queue.some((r) => r.held);
  // Continue picks up where the stopped turn left off: the held queue first, else a nudge.
  const resume = useMutation({
    mutationFn: () => held ? api.t3QueueResume(v.id) : api.t3Dispatch(v.id, { text: "Continúa por donde lo dejaste.", mode: "auto" }),
    onSuccess: () => qc.invalidateQueries({ queryKey }),
  });
  // Stopped with an empty box: the send button continues where it stopped.
  const resumable = !busy && (held || v.interrupted) && !text.trim() && !files.length;
  const canSend = (!!text.trim() || ready.length > 0 || resumable) && !uploading && !send.isPending && !resume.isPending;
  const canNext = !!question && !question.busy && (!!text.trim() || question.picked);
  const submit = (asSteer?: boolean) => {
    // An open question takes the typed text as its free answer, or its checked options.
    if (question) { if (canNext) { question.next(text.trim()); setText(""); } return; }
    if (!canSend) return;
    if (resumable) resume.mutate(); else send.mutate(busy ? asSteer : undefined);
  };

  return (
    <form onSubmit={(e) => { e.preventDefault(); submit(); }}
      onDragOver={(e) => { if (e.dataTransfer.types.includes("Files")) e.preventDefault(); }}
      onDrop={(e) => { if (e.dataTransfer.files.length) { e.preventDefault(); addFiles(e.dataTransfer.files); } }}
      onPaste={(e) => {
        // Screenshots often come only as clipboard items, not in .files.
        const fs = Array.from(e.clipboardData.items).flatMap((i) => { const f = i.kind === "file" ? i.getAsFile() : null; return f ? [f] : []; });
        if (fs.length) { e.preventDefault(); addFiles(fs); }
      }}
      className="relative rounded-3xl border border-slate-200 bg-white shadow-md dark:border-border-strong dark:bg-surface">
      {suggest.menu}
      {files.length > 0 && (
        <div className="flex flex-wrap gap-2 px-3 pt-3">
          {files.map((f) => (
            <div key={f.key} title={f.error ?? f.name} className={`group relative size-14 shrink-0 overflow-hidden rounded-xl border bg-slate-50 dark:bg-raised ${f.error ? "border-red-400" : "border-slate-200 dark:border-border"}`}>
              <button type="button" onClick={() => setViewing({ src: f.preview, name: f.name, mime: f.mime })} aria-label={`Ver ${f.name}`} className="block size-14 cursor-zoom-in">
                {f.mime.startsWith("image/") ? <img src={f.preview} alt="" className="size-14 shrink-0 object-cover" /> : isPdf(f) ? <PdfThumb src={f.preview} width={56} className="size-14" /> : <span className="grid size-14 shrink-0 place-items-center text-slate-400"><FileText className="size-6" /></span>}
              </button>
              {f.error && <span className="absolute inset-x-0 bottom-0 bg-red-500 text-center text-[9px] font-medium text-white">Error</span>}
              {!f.done && !f.error && <span className="absolute inset-0 grid place-items-center bg-white/60 dark:bg-black/40"><span className="size-4 animate-spin rounded-full border-2 border-current border-t-transparent" /></span>}
              <button type="button" onClick={() => removeFile(f.key)} aria-label={`Quitar ${f.name}`} className="absolute right-1 top-1 grid size-5 place-items-center rounded-full bg-slate-900/70 text-white transition-opacity hover:bg-slate-900 sm:opacity-0 sm:group-hover:opacity-100 sm:focus-visible:opacity-100"><X weight="regular" className="size-3" /></button>
            </div>
          ))}
        </div>
      )}
      {viewing && <AttachmentPreview {...viewing} onClose={() => setViewing(null)} />}
      <div className="relative">
        {/* Inline @file chips: this layer paints the text under a see-through textarea, glyph for glyph, so the caret stays aligned. */}
        {ranges.length > 0 && <div ref={paint} aria-hidden="true" className="pointer-events-none absolute inset-0 overflow-hidden whitespace-pre-wrap break-words px-4 pb-1 pt-3.5 text-[15px] leading-6">
          {paintTokens(text, tokens)}{"\u200b"}
        </div>}
        <textarea ref={area} rows={1} value={text} onChange={(e) => { setText(e.target.value); suggest.track(); }} onSelect={() => { snapSelection(); suggest.track(); }} onClick={suggest.track} aria-label="Mensaje"
          onScroll={(e) => { if (paint.current) paint.current.scrollTop = e.currentTarget.scrollTop; }}
          placeholder={question ? "Escribe tu respuesta a la pregunta…" : "Escribe al hilo…"}
          onKeyDown={(e) => { if (e.nativeEvent.isComposing || suggest.onKeyDown(e) || deleteToken(e)) return; if (e.key === "Enter" && !e.shiftKey) { e.preventDefault(); submit(e.ctrlKey || e.metaKey); } }}
          className={`relative block max-h-60 w-full resize-none break-words bg-transparent px-4 pb-1 pt-3.5 text-[15px] leading-6 !outline-none placeholder:text-slate-400 ${ranges.length ? "text-transparent caret-slate-900 dark:caret-white" : ""}`} />
      </div>
      <div className="flex items-center gap-0.5 px-2 pb-2">
        <div className="flex min-w-0 flex-1 flex-wrap items-center gap-0.5">
          <input ref={picker} type="file" multiple hidden onChange={(e) => { if (e.target.files) addFiles(e.target.files); e.target.value = ""; }} />
          <button type="button" onClick={() => picker.current?.click()} title="Adjuntar imágenes o ficheros" aria-label="Adjuntar"
            className="grid size-8 place-items-center rounded-full text-slate-500 hover:bg-slate-100 hover:text-slate-900 dark:text-text-secondary dark:hover:bg-raised dark:hover:text-white">
            <Paperclip weight="regular" className="size-4" />
          </button>
          <ModelPicker models={models} selected={selected} effort={effortValue}
            effortsOf={(value) => { const [pid, sl] = value.split("|"); const m = catalog.data?.providers.find((p) => p.instance_id === pid)?.models.find((x) => x.slug === sl); return m?.effort ? { values: m.effort.values, initial: value === selected ? effortValue : savedEffort(sl, m.effort.values) || m.effort.default } : undefined; }}
            onChange={(value, eff) => { setChoice(value); setEffort(eff); const sl = value.split("|")[1]; if (eff) saveEffort(sl, eff); }} />
          <Chip title="Permisos" value={v.runtime_mode} options={runtimeModes} onChange={(x) => settings.mutate({ runtime_mode: x })} />
        </div>
        {question ? (
          <>
            {busy && (
              <button type="button" onClick={() => stop.mutate()} disabled={stop.isPending} title="Parar el turno en curso" aria-label="Parar"
                className="grid size-9 shrink-0 place-items-center rounded-full border border-slate-200 text-slate-700 transition hover:bg-slate-100 disabled:opacity-50 dark:border-border dark:text-text-secondary dark:hover:bg-raised">
                <Stop weight="fill" className="size-3.5" />
              </button>
            )}
            <button type="submit" disabled={!canNext} title={question.last ? "Responder (Enter)" : "Siguiente (Enter)"}
              className="ml-1 inline-flex h-9 shrink-0 items-center gap-1 rounded-full bg-slate-900 px-4 text-sm font-medium text-white transition hover:bg-slate-700 disabled:bg-slate-200 disabled:text-slate-400 dark:bg-white dark:text-slate-900 dark:hover:bg-slate-200 dark:disabled:bg-raised dark:disabled:text-muted">
              {question.last ? "Responder" : "Siguiente"}<CaretRight weight="regular" className="size-3.5" />
            </button>
          </>
        ) : busy && !text.trim() ? (
          <button type="button" onClick={() => stop.mutate()} disabled={stop.isPending} title="Parar el turno en curso" aria-label="Parar"
            className="grid size-9 shrink-0 place-items-center rounded-full bg-slate-900 text-white transition hover:bg-slate-700 disabled:opacity-50 dark:bg-white dark:text-slate-900 dark:hover:bg-slate-200">
            <Stop weight="fill" className="size-3.5" />
          </button>
        ) : <>{busy && <span className="mr-2 hidden items-center gap-1 text-[11px] text-slate-400 dark:text-muted sm:inline-flex" title="Ctrl+Enter inyecta el mensaje en el turno en curso; Enter lo pone en cola"><Lightning className="size-3" aria-hidden="true" /><kbd className="font-sans">Ctrl+Enter</kbd></span>}<button type="submit" disabled={!canSend} title={resumable ? "Continuar por donde iba" : busy ? "Encolar (Enter) · Ctrl+Enter: steer" : "Enviar (Enter)"} aria-label={resumable ? "Continuar" : "Enviar"}
          className="grid size-9 shrink-0 place-items-center rounded-full bg-slate-900 p-0 text-white transition hover:bg-slate-700 disabled:bg-slate-200 disabled:text-slate-400 dark:bg-white dark:text-slate-900 dark:hover:bg-slate-200 dark:disabled:bg-raised dark:disabled:text-muted">
          {send.isPending || resume.isPending ? <span className="size-4 animate-spin rounded-full border-2 border-current border-t-transparent" /> : resumable ? <Play weight="fill" className="block size-3.5 translate-x-px" /> : busy ? <ListBullets weight="bold" className="block size-4" /> : <ArrowUp weight="bold" className="block size-4" />}
        </button></>}
      </div>
      {(send.error || settings.error || stop.error || resume.error) && <div className="px-4 pb-2"><ErrorText error={send.error ?? settings.error ?? stop.error ?? resume.error} /></div>}
    </form>
  );
}

// prOf reads owner/repo and number from a pull request link.
const prOf = (url: string) => {
  const m = url.match(/github\.com\/([^/]+\/[^/]+)\/pull\/(\d+)/);
  return m ? { repo: m[1], number: Number(m[2]) } : undefined;
};

// Changes shows the thread's own changes or its pull request, and sends a
// selected range to the chat through the same queue or steer as the composer.
function Changes({ v, queryKey }: { v: T3ThreadView; queryKey: unknown[] }) {
  const qc = useQueryClient();
  const prs = v.pull_requests.map((p) => ({ ...p, parsed: prOf(p.url) })).filter((p) => p.parsed);
  const [source, setSource] = useState("thread");
  const pr = prs.find((p) => p.url === source)?.parsed;
  const local = useQuery({ queryKey: ["t3-diff", v.id, v.messages.length], queryFn: () => api.t3Diff(v.id), enabled: !pr });
  const prDiff = useQuery({ queryKey: ["github-diff", pr?.repo, pr?.number], queryFn: () => api.githubDiff(pr!.repo, pr!.number), enabled: !!pr });
  const text = pr ? prDiff.data?.diff : local.data?.diff;
  const suggest = async (s: Suggestion) => {
    const where = s.start === s.end ? `línea ${s.start}` : `líneas ${s.start}–${s.end}`;
    const body = `Sugerencia sobre \`${s.path}\` (${where}):\n\n\`\`\`diff\n${s.snippet}\n\`\`\`\n\n${s.question}`;
    await api.t3Dispatch(v.id, { text: body, mode: s.steer ? "steer" : "auto" });
    qc.invalidateQueries({ queryKey });
  };
  return (
    <section className="flex min-h-0 flex-1 flex-col gap-2">
      <div className="flex shrink-0 flex-wrap items-center gap-2">
        <Select className="w-64" value={source} onChange={setSource} options={[
          { value: "thread", label: "Cambios del hilo (worktree)" },
          ...prs.map((p) => ({ value: p.url, label: `PR #${p.number} · ${p.repository}` })),
        ]} />
        {v.worktree_path && <span className="truncate font-mono text-xs text-slate-500 dark:text-muted" title={v.worktree_path}>{v.worktree_path}</span>}
      </div>
      <ErrorText error={pr ? prDiff.error : local.error} />
      {text === undefined ? <Notice loading /> : text === "" ? <p className="text-sm text-slate-500 dark:text-muted">Sin cambios todavía.</p>
        : <div className="min-h-0 flex-1"><Diff key={source} text={text} onSuggest={suggest} fill /></div>}
    </section>
  );
}

type Shown = { src: string; name: string; mime: string };
const isPdf = (f: { mime: string; name: string }) => f.mime === "application/pdf" || /\.pdf$/i.test(f.name);

// AttachmentPreview shows an image, PDF or text full size; anything else offers a download.
function AttachmentPreview({ src, name, mime, onClose }: Shown & { onClose: () => void }) {
  useEffect(() => {
    const esc = (e: KeyboardEvent) => e.key === "Escape" && onClose();
    window.addEventListener("keydown", esc);
    return () => window.removeEventListener("keydown", esc);
  }, [onClose]);
  const inline = mime === "application/pdf" || mime.startsWith("text/") || /\.html?$/i.test(name) || /\.(pdf|txt|md|log|json)$/i.test(name);
  return createPortal(
    <div role="dialog" aria-label={name} className="notif-in fixed inset-0 z-[120] flex flex-col bg-black/80 p-3 sm:p-6" onMouseDown={(e) => e.target === e.currentTarget && onClose()}>
      <div className="mb-2 flex items-center gap-2 text-sm text-white">
        <span className="min-w-0 flex-1 truncate">{name}</span>
        <a href={src} download={name} className="rounded-md px-2 py-1 hover:bg-white/10">Descargar</a>
        <button type="button" onClick={onClose} aria-label="Cerrar" className="grid size-8 place-items-center rounded-md hover:bg-white/10"><X weight="regular" className="size-5" /></button>
      </div>
      <div className="flex min-h-0 flex-1 items-center justify-center" onMouseDown={(e) => e.target === e.currentTarget && onClose()}>
        {mime.startsWith("image/") ? <img src={src} alt={name} className="max-h-full max-w-full rounded-md object-contain shadow-2xl" />
          : inline ? <iframe src={src} title={name} sandbox={mime === "text/html" ? "allow-scripts allow-popups allow-popups-to-escape-sandbox" : undefined} className="size-full rounded-md bg-white" />
          : <p className="text-sm text-white/80">No hay vista previa para este tipo de fichero.</p>}
      </div>
    </div>,
    document.body,
  );
}

// SentAttachments shows the files sent with a user message; a click opens the preview.
function SentAttachments({ threadId, list }: { threadId: string; list: T3Attachment[] }) {
  const [viewing, setViewing] = useState<Shown | null>(null);
  const url = (a: T3Attachment) => `/api/t3/threads/${encodeURIComponent(threadId)}/attachments/${encodeURIComponent(a.id)}?${new URLSearchParams({ name: a.name, mime: a.mime_type })}`;
  return (
    <div className="mb-1 flex max-w-[85%] flex-wrap justify-end gap-1.5">
      {list.map((a) => {
        const shown = { src: url(a), name: a.name, mime: a.mime_type };
        if (isPdf({ mime: a.mime_type, name: a.name })) return (
          <button key={a.id} type="button" onClick={() => setViewing(shown)} title={a.name} aria-label={`Ver ${a.name}`}
            className="w-36 cursor-zoom-in overflow-hidden rounded-xl border border-slate-200 bg-white text-left dark:border-border dark:bg-surface">
            <PdfThumb src={shown.src} width={144} className="h-40 w-36 border-b border-slate-200 dark:border-border" />
            <span className="flex items-center gap-1.5 px-2 py-1.5 text-xs"><FileText className="size-3.5 shrink-0 text-red-600" /><span className="min-w-0 truncate">{a.name}</span></span>
          </button>
        );
        return a.type === "image" ? (
          <button key={a.id} type="button" onClick={() => setViewing(shown)} title={a.name} aria-label={`Ver ${a.name}`}
            className="cursor-zoom-in overflow-hidden rounded-xl border border-slate-200 bg-slate-50 dark:border-border dark:bg-raised">
            <img src={shown.src} alt={a.name} loading="lazy" className="block max-h-48 max-w-[16rem] object-cover" />
          </button>
        ) : (
          <button key={a.id} type="button" onClick={() => setViewing(shown)} title={a.name}
            className="flex max-w-[16rem] items-center gap-2 rounded-xl border border-slate-200 bg-white px-3 py-2 text-left text-sm hover:bg-slate-50 dark:border-border dark:bg-surface dark:hover:bg-raised">
            <FileText className="size-5 shrink-0 text-slate-500" /><span className="min-w-0 truncate">{a.name}</span>
          </button>
        );
      })}
      {viewing && <AttachmentPreview {...viewing} onClose={() => setViewing(null)} />}
    </div>
  );
}

// Handoff marks where the thread passed its context to another model.
function Handoff({ m }: { m: T3ViewMessage }) {
  const chip = (model?: string, provider?: string) => (
    <span className="inline-flex max-w-[12rem] items-center gap-1.5 rounded-full border border-slate-200 bg-white px-2 py-0.5 normal-case tracking-normal text-slate-600 dark:border-border dark:bg-surface dark:text-text-secondary">
      <ProviderIcon driver={provider ?? ""} model={model} /><span className="truncate font-mono">{model || "—"}</span>
    </span>
  );
  return (
    <div role="separator" aria-label={`Cambio de modelo: de ${m.from || "otro modelo"} a ${m.to || "otro modelo"}`} title="El hilo pasó su contexto a otro modelo"
      className="flex items-center gap-2 text-[11px] font-medium uppercase tracking-wide text-slate-400">
      <span className="h-px flex-1 bg-slate-200 dark:bg-border" />
      {m.from && chip(m.from, m.from_provider)}
      <ArrowRight weight="bold" className="size-3.5 shrink-0" />
      {chip(m.to, m.to_provider)}
      <span className="h-px flex-1 bg-slate-200 dark:bg-border" />
    </div>
  );
}

// HtmlRender shows a page the agent rendered with T3's html_render tool, as T3
// Code does: inline, sandboxed (its scripts run in an opaque origin), with a full view.
function HtmlRender({ threadId, page }: { threadId: string; page: NonNullable<T3ViewMessage["html"]> }) {
  const [full, setFull] = useState(false);
  const name = `${page.title || "Página"}.html`;
  const src = `/api/t3/threads/${encodeURIComponent(threadId)}/attachments/${encodeURIComponent(page.attachment_id)}?${new URLSearchParams({ name, mime: "text/html" })}`;
  // The page reports its real height (the proxy adds a script with ?fit=1); the
  // agent's own height only holds until then. No inner scroll: the frame grows to fit.
  const frame = useRef<HTMLIFrameElement>(null);
  const [measured, setMeasured] = useState(0);
  useEffect(() => {
    const on = (e: MessageEvent) => {
      const h = (e.data as { doryFrameHeight?: unknown } | null)?.doryFrameHeight;
      if (e.source === frame.current?.contentWindow && typeof h === "number" && h > 0) setMeasured(Math.min(4000, Math.ceil(h)));
    };
    window.addEventListener("message", on);
    return () => window.removeEventListener("message", on);
  }, []);
  const height = measured || Math.min(1200, Math.max(80, Math.round(page.height || 400)));
  return (
    <figure className="overflow-hidden rounded-xl border border-slate-200 bg-white dark:border-border dark:bg-surface">
      <figcaption className="flex items-center gap-2 border-b border-slate-200 px-3 py-1.5 text-xs dark:border-border">
        <Browser className="size-4 text-slate-400" /><span className="min-w-0 flex-1 truncate font-medium">{page.title || "Página"}</span>
        <button type="button" onClick={() => setFull(true)} title="Ver a pantalla completa" aria-label="Ver a pantalla completa" className={iconBtn}><ArrowsOut className="size-4" /></button>
        <a href={src} target="_blank" rel="noreferrer" title="Abrir en otra pestaña" aria-label="Abrir en otra pestaña" className={iconBtn}><ArrowSquareOut className="size-4" /></a>
      </figcaption>
      <iframe ref={frame} src={`${src}&fit=1`} title={page.title || "Página"} sandbox="allow-scripts allow-popups allow-popups-to-escape-sandbox" loading="lazy" scrolling="no" style={{ height }} className="block w-full overflow-hidden bg-white" />
      {full && <AttachmentPreview src={src} name={name} mime="text/html" onClose={() => setFull(false)} />}
    </figure>
  );
}

const secretStates: Record<string, string> = { pending: "pendiente", saved: "guardado", declined: "rechazado", cancelled: "cancelado" };

// SecretLine marks a secret request in the conversation, without its value.
function SecretLine({ m }: { m: T3ViewMessage }) {
  const tone = m.status === "saved" ? "text-green-700 dark:text-green-400" : m.status === "pending" ? "text-amber-600 dark:text-amber-400" : "text-slate-400";
  return (
    <div className="flex items-center gap-2 text-xs text-slate-500 dark:text-muted" title={m.text}>
      <Key className={`size-4 shrink-0 ${tone}`} />
      <span className="min-w-0 truncate">Secreto <span className="font-medium text-slate-700 dark:text-text-secondary">{m.title}</span></span>
      <span className={tone}>· {secretStates[m.status ?? ""] ?? m.status}</span>
    </div>
  );
}

// SecretRequest answers request_secret: the value goes to T3, which hands the
// agent a one-use reference; it never enters the chat or Dory's storage.
function SecretRequest({ threadId, m, onDone }: { threadId: string; m: T3ViewMessage; onDone: () => void }) {
  const [value, setValue] = useState("");
  const [shown, setShown] = useState(false);
  const answer = useMutation({
    mutationFn: (decline: boolean) => api.t3AnswerSecret(threadId, m.id, decline ? { decline: true } : { secret: value }),
    onSuccess: () => { setValue(""); onDone(); },
  });
  const btn = "inline-flex h-8 items-center rounded-full px-4 text-sm font-medium transition disabled:opacity-50";
  return (
    <form className="space-y-3" onSubmit={(e) => { e.preventDefault(); if (value.trim()) answer.mutate(false); }}>
      <div>
        <p className="text-sm font-medium text-slate-900 dark:text-white">{m.title}</p>
        {m.text && <p className="mt-0.5 text-sm text-slate-500 dark:text-muted">{m.text}</p>}
      </div>
      <div className="flex items-center gap-1 rounded-xl border border-slate-300 bg-white pr-1 focus-within:border-slate-500 dark:border-border-strong dark:bg-surface">
        <input autoFocus type={shown ? "text" : "password"} value={value} onChange={(e) => setValue(e.target.value)} placeholder={m.detail || "Pega el valor"}
          autoComplete="off" autoCorrect="off" autoCapitalize="off" spellCheck={false} aria-label={m.title}
          className="min-h-10 w-full bg-transparent px-3 font-mono text-sm !outline-none placeholder:font-sans placeholder:text-slate-400" />
        <button type="button" onClick={() => setShown(!shown)} title={shown ? "Ocultar" : "Mostrar"} aria-label={shown ? "Ocultar" : "Mostrar"} className={iconBtn}>
          {shown ? <EyeSlash className="size-4" /> : <Eye className="size-4" />}
        </button>
      </div>
      {answer.error && <ErrorText error={answer.error} />}
      <div className="flex justify-end gap-2">
        <button type="button" disabled={answer.isPending} onClick={() => answer.mutate(true)} className={`${btn} text-slate-600 hover:bg-slate-100 dark:text-text-secondary dark:hover:bg-raised`}>Rechazar</button>
        <button type="submit" disabled={answer.isPending || !value.trim()} className={`${btn} bg-slate-900 text-white hover:bg-slate-700 dark:bg-white dark:text-slate-900 dark:hover:bg-slate-200`}>{answer.isPending ? "Enviando…" : "Guardar"}</button>
      </div>
    </form>
  );
}


// tokenize marks the tokens of a sent message as tok:kind:value links: its
// @files and $skills (from T3's context records), a leading /command and #PRs.
const escapeRe = (s: string) => s.replace(/[.*+?^${}()|[\]\\]/g, "\\$&");
function tokenize(m: T3ViewMessage): string {
  const link = (kind: string, value: string) => `[${kind}](tok:${kind}:${encodeURIComponent(value)})`;
  // Quotes sent from the composer come back as one object each.
  let t = m.text.replace(QUOTE_RE, (_, label: string, body: string) =>
    `\n\n${link("quote", JSON.stringify({ label, body: body.split("\n").map((l) => l.replace(/^> ?/, "")).join("\n").trim() }))}\n\n`);
  for (const p of m.mentions ?? []) t = t.split(`@${p}`).join(link("path", p));
  for (const k of m.skills ?? []) t = t.replace(new RegExp(`(^|\\s)\\$${escapeRe(k)}(?=\\s|$)`, "g"), (_, pre: string) => pre + link("skill", k));
  t = t.replace(/^\/([\w:.-]+)(?=\s|$)/, (_, c: string) => link("command", c));
  t = t.replace(/(^|\s)#(\d+)(?=\s|$|[.,;:])/g, (_, pre: string, n: string) => pre + link("ref", n));
  return t;
}
const keepToken = (url: string) => (url.startsWith("tok:") || url.startsWith("file://") ? url : defaultUrlTransform(url));
const tokenLinks = {
  a: ({ href, children, node: _n, ...rest }: React.AnchorHTMLAttributes<HTMLAnchorElement> & { node?: unknown }) => {
    if (!href?.startsWith("tok:")) return <RefLink href={href} {...rest}>{children}</RefLink>;
    const [, kind, raw] = href.split(":");
    const value = decodeURIComponent(raw ?? "");
    if (kind === "quote") {
      const q = (() => { try { return JSON.parse(value) as { label: string; body: string }; } catch { return { label: value, body: value }; } })();
      return <TokenChip kind="quote" label={q.label} title={q.body} />;
    }
    const label = kind === "path" ? value.replace(/\/+$/, "").split("/").pop() || value : value;
    return <TokenChip kind={kind as Token["kind"]} label={label} title={kind === "ref" ? `#${value}` : value} />;
  },
};
