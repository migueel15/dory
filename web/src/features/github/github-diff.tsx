import { ArrowsIn,ArrowsOut,Columns,Rows,TreeView } from "@phosphor-icons/react";
import { useMutation,useQueryClient } from "@tanstack/react-query";
import { useVirtualizer } from "@tanstack/react-virtual";
import { createContext,memo,useCallback,useContext,useDeferredValue,useEffect,useMemo,useRef,useState } from "react";
import { createPortal } from "react-dom";
import type { LaunchChoice } from "../../api/client";
import { api } from "../../api/client";
import { useAskPullAgent,useGithubAct,useGithubDiff,usePullAgent,usePullReviewMark,usePullReviews } from "../../api/queries/github";
import { qk } from "../../api/queries/keys";
import type { FileRef } from "../../components/refs";
import { ErrorText,Input } from "../../components/ui";
import { highlight,languageOf,loadLanguage } from "../../highlight";
import { storage,storageKeys } from "../../lib/storage";
import { File,Pick,Review,Row,parseDiff,patchHash,splitRows,tree } from "./diff-parse";
import { Choose,CommentForm,Extend,FileHead,Hl,SplitRow,Suggestion,Tree,UnifiedRow,number,panel,side } from "./diff-parts";
import { ReviewLaunch } from "./diff-review";

type Item =
  | { key: string; kind: "head"; fi: number }
  | { key: string; kind: "end"; fi: number }
  | { key: string; kind: "line"; fi: number; i: number }
  | { key: string; kind: "pair"; fi: number; r: Row }
  | { key: string; kind: "form"; fi: number; whole: boolean };
const estimate = { head: 36, end: 12, line: 20, pair: 20, form: 150 };

// Diff shows a pull request diff, or the given diff text when onSuggest sends
// selections to a chat; GitHub reviews and comments only apply to the first.
// FullscreenHost is the element a diff fills in full screen, instead of the window.
export const FullscreenHost = createContext<HTMLElement | null>(null);

// fill makes the diff take its container's height instead of 80vh.
// onAsked runs after a question reaches the pull request's review thread.
// focus opens a file on a line (or range) and scrolls to it, each time its seq changes.
// Only the rows near the viewport are in the DOM; search and jumps work on the parsed files.
export const Diff = memo(function Diff({ repo = "", pr = 0, text, onSuggest, steerable = true, onAsked, focus, fill = false, toolsHost }: { repo?: string; pr?: number; text?: string; onSuggest?: (s: Suggestion) => Promise<void>; steerable?: boolean; onAsked?: () => void; focus?: FileRef & { seq: number }; fill?: boolean; toolsHost?: HTMLElement | null }) {
  const qc = useQueryClient();
  const gh = !onSuggest && !!repo && !!pr;
  const diff = useGithubDiff(repo, pr, { enabled: text === undefined });
  const source = text ?? diff.data?.diff ?? "";
  const files = useMemo(() => parseDiff(source), [source]);
  const [filter, setFilter] = useState("");
  const [pick, setPick] = useState<Pick>();
  const draft = useRef(""); // text of the open comment form
  const [drag, setDrag] = useState<{ file: string; from: number }>();
  const [open, setOpen] = useState(false);
  const [full, setFull] = useState(false);
  // The last chosen layout is remembered in this browser for every diff.
  // In a side panel (fill) the file explorer starts closed to leave room for the diff.
  const [panelOpen, setPanelOpen] = useState(!fill);
  const [split, setSplitState] = useState(() => { try { return storage.get(storageKeys.diffView) === "split"; } catch { return false; } });
  const setSplit = (f: (v: boolean) => boolean) => setSplitState((v) => {
    const next = f(v);
    try { storage.set(storageKeys.diffView, next ? "split" : "unified"); } catch { /* storage unavailable */ }
    return next;
  });
  const [loaded, setLoaded] = useState(0); // bumps when a language finishes loading
  useEffect(() => {
    if (!full) return;
    const esc = (e: KeyboardEvent) => e.key === "Escape" && setFull(false);
    window.addEventListener("keydown", esc);
    // Lock the page behind so only the diff scrolls.
    const html = document.documentElement, prev = [html.style.overflow, document.body.style.overflow];
    html.style.overflow = document.body.style.overflow = "hidden";
    return () => {
      window.removeEventListener("keydown", esc);
      [html.style.overflow, document.body.style.overflow] = prev;
    };
  }, [full]);
  useEffect(() => {
    const langs = new Set(files.map((f) => languageOf(f.path)).filter((l): l is string => !!l));
    langs.forEach((l) => loadLanguage(l).then((ok) => ok && setLoaded((n) => n + 1)));
  }, [files]);
  // Highlighting is lazy: a line is highlighted the first time its row renders.
  const hl = useMemo<Hl>(() => {
    const cache = new Map<string, string | undefined>();
    return (path, i, text) => {
      const key = `${path}:${i}`;
      if (cache.has(key)) return cache.get(key);
      const lang = languageOf(path);
      const out = lang ? highlight(text.slice(1), lang) : undefined;
      cache.set(key, out);
      return out;
    };
  }, [loaded]); // eslint-disable-line react-hooks/exhaustive-deps
  useEffect(() => {
    const stop = () => setDrag((d) => { if (d) setOpen(true); return undefined; });
    window.addEventListener("mouseup", stop);
    return () => window.removeEventListener("mouseup", stop);
  }, []);
  // Review marks live in Dory and only count while the file patch is unchanged.
  const [hashes, setHashes] = useState(new Map<string, string>());
  useEffect(() => {
    let live = true;
    Promise.all(files.map(async (f) => [f.path, await patchHash(f)] as const)).then((h) => live && setHashes(new Map(h)));
    return () => { live = false; };
  }, [files]);
  const reviews = usePullReviews(repo, pr, { enabled: gh });
  const reviewed = useMemo(() => new Set((reviews.data ?? []).filter((v) => v.patch_hash === hashes.get(v.path)).map((v) => v.path)), [reviews.data, hashes]);
  const mark = usePullReviewMark(repo, pr, (path) => hashes.get(path)!, {
    onSettled: () => qc.invalidateQueries({ queryKey: qk.pullReviews(repo, pr) }),
  });
  // Reviewed files fold unless the user opens them.
  const [folded, setFolded] = useState(new Map<string, boolean>());
  const isFolded = (path: string) => folded.get(path) ?? reviewed.has(path);
  // Latest state for callbacks that must keep a stable identity, so the memoized rows do not re-render.
  const now = useRef({ pick, drag, reviewed, hashes, mark: mark.mutate, at: new Map<string, number>() });
  now.current = { ...now.current, pick, drag, reviewed, hashes, mark: mark.mutate };
  const scroller = useRef<HTMLDivElement>(null);
  // Focus a referenced file and line: unfold it, highlight the lines and scroll there.
  const focused = useRef(0);
  const want = useRef<{ path: string; i: number }>(undefined);
  const [, wake] = useState(0);
  useEffect(() => {
    if (!focus || focused.current === focus.seq || !files.length) return;
    focused.current = focus.seq;
    const f = files.find((x) => x.path === focus.path) ?? files.find((x) => x.path.endsWith(`/${focus.path}`) || focus.path.endsWith(`/${x.path}`));
    if (!f) return;
    setFolded((m) => new Map(m).set(f.path, false));
    setOpen(false);
    const at = (n: number) => f.lines.findIndex((l) => l.kind !== "hunk" && l.kind !== "del" && l.new === n);
    const start = focus.line ? at(focus.line) : -1;
    const end = focus.end ? Math.max(start, at(focus.end)) : start;
    setPick(start >= 0 ? { file: f.path, start, end } : undefined);
    want.current = { path: f.path, i: start };
    wake((n) => n + 1);
  }, [focus, files]); // eslint-disable-line react-hooks/exhaustive-deps
  const toggleReview = useCallback((path: string) => {
    const n = now.current;
    if (!n.hashes.has(path)) return;
    setFolded((m) => { const x = new Map(m); x.delete(path); return x; });
    n.mark({ path, on: !n.reviewed.has(path) });
  }, []);
  const toggleFold = useCallback((path: string) => setFolded((m) => new Map(m).set(path, !(m.get(path) ?? now.current.reviewed.has(path)))), []);
  const commentFile = useCallback((path: string) => { setPick({ file: path, start: -1, end: -1 }); setOpen(true); }, []);
  const agent = usePullAgent(repo, pr, { enabled: gh });
  // The first question creates the review thread: it waits in launching while
  // the user picks the machine and model.
  const [launching, setLaunching] = useState<{ f: File; p: Pick; body: string }>();
  const done = () => { setPick(undefined); draft.current = ""; };
  const ask = useAskPullAgent(repo, pr, ({ f, p, body, launch }: { f: File; p: Pick; body: string; launch?: LaunchChoice }) => {
    // A whole-file question goes with lines 0–0 and no snippet.
    if (p.start < 0) return { path: f.path, start: 0, end: 0, snippet: "", question: body.trim(), launch };
    const lines = f.lines.slice(p.start, p.end + 1).filter((l) => l.kind !== "hunk");
    const nums = lines.map(number);
    return { path: f.path, start: Math.min(...nums), end: Math.max(...nums), snippet: lines.map((l) => l.text).join("\n"), question: body.trim(), launch };
  }, {
    onSuccess: () => { done(); setLaunching(undefined); qc.invalidateQueries({ queryKey: qk.pullAgent(repo, pr) }); onAsked?.(); },
  });
  const askOrLaunch = (f: File, p: Pick, body: string) => (agent.data?.available ? ask.mutate({ f, p, body }) : setLaunching({ f, p, body }));
  const suggest = useMutation({
    mutationFn: ({ f, p, body, steer }: { f: File; p: Pick; body: string; steer: boolean }) => {
      const lines = f.lines.slice(p.start, p.end + 1).filter((l) => l.kind !== "hunk");
      const nums = lines.map(number);
      return onSuggest!({ path: f.path, start: Math.min(...nums), end: Math.max(...nums), snippet: lines.map((l) => l.text).join("\n"), question: body.trim(), steer });
    },
    onSuccess: done,
  });
  const comment = useGithubAct(repo, "pulls", pr, (a: Parameters<typeof api.githubAct>[3]) => a, {
    onSuccess: () => {
      done();
      qc.invalidateQueries({ queryKey: qk.githubDetail(repo, "pulls", pr) });
    },
  });
  // The search matches file paths and the text of the changed lines; typing stays responsive
  // because the filtering follows a deferred copy of the term.
  const deferred = useDeferredValue(filter);
  const q = deferred.trim().toLowerCase();
  const hits = useMemo(() => new Map(files.map((f) => [f.path, q ? f.lines.filter((l) => l.kind !== "hunk" && l.text.slice(1).toLowerCase().includes(q)).length : 0])), [files, q]);
  const shown = useMemo(() => files.filter((f) => !q || f.path.toLowerCase().includes(q) || hits.get(f.path)! > 0), [files, q, hits]);

  // On touch screens a drag scrolls; tapping a second line number extends the selection instead.
  const touch = useRef(false);
  const choose = useCallback<Choose>((file, i, shift) => {
    const { pick } = now.current;
    const from = shift && pick?.file === file && pick.start >= 0 ? pick.start : i;
    setPick({ file, start: Math.min(from, i), end: Math.max(from, i) });
    // A tap has no drag: the comment box moves when the range changes, and a drag
    // would keep extending to whatever line slides under the finger.
    if (touch.current) { setDrag(undefined); setOpen(true); return; }
    setOpen(false);
    setDrag({ file, from });
  }, []);
  const extend = useCallback<Extend>((file, i) => {
    const { drag, pick } = now.current;
    if (drag?.file !== file) return;
    const start = Math.min(drag.from, i), end = Math.max(drag.from, i);
    if (pick?.file === file && pick.start === start && pick.end === end) return;
    setPick({ file, start, end });
  }, []);
  const publish = (f: File, p: Pick, body: string) => {
    if (p.start < 0) return comment.mutate({ type: "comment_file", body, path: f.path });
    const first = f.lines[p.start], last = f.lines[p.end];
    comment.mutate({ type: "comment_lines", body, path: f.path, line: number(last), side: side(last), start_line: number(first), start_side: side(first) });
  };

  // The flat list the virtual scroller shows: file headers, lines (or split pairs), the open form and file ends.
  const form = open && !drag && pick && pick.start >= 0 ? pick : undefined;
  const whole = pick && pick.start < 0 ? pick.file : undefined;
  const splits = useMemo(() => (split ? new Map(shown.map((f) => [f.path, splitRows(f.lines)])) : undefined), [shown, split]);
  const { items, at } = useMemo(() => {
    const out: Item[] = [];
    const at = new Map<string, number>(); // path → header row, path:i → line row
    shown.forEach((f, fi) => {
      at.set(f.path, out.length);
      out.push({ key: `h:${f.path}`, kind: "head", fi });
      if (whole === f.path) out.push({ key: "form", kind: "form", fi, whole: true });
      if (!(folded.get(f.path) ?? reviewed.has(f.path))) {
        if (splits) {
          splits.get(f.path)!.forEach((r, k) => {
            if (r.left !== undefined) at.set(`${f.path}:${r.left}`, out.length);
            if (r.right !== undefined) at.set(`${f.path}:${r.right}`, out.length);
            out.push({ key: `s:${f.path}:${k}`, kind: "pair", fi, r });
            if (form?.file === f.path && (r.left === form.end || r.right === form.end)) out.push({ key: "form", kind: "form", fi, whole: false });
          });
        } else {
          f.lines.forEach((_, i) => {
            at.set(`${f.path}:${i}`, out.length);
            out.push({ key: `l:${f.path}:${i}`, kind: "line", fi, i });
            if (form?.file === f.path && form.end === i) out.push({ key: "form", kind: "form", fi, whole: false });
          });
        }
      }
      out.push({ key: `e:${f.path}`, kind: "end", fi });
    });
    return { items: out, at };
  }, [shown, splits, folded, reviewed, whole, form?.file, form?.end]); // eslint-disable-line react-hooks/exhaustive-deps
  now.current.at = at;
  const virt = useVirtualizer({
    count: items.length, getScrollElement: () => scroller.current, overscan: 20,
    estimateSize: (i) => estimate[items[i].kind], getItemKey: (i) => items[i]?.key ?? i,
  });
  useEffect(() => {
    const w = want.current;
    if (!w) return;
    want.current = undefined;
    const idx = at.get(w.i >= 0 ? `${w.path}:${w.i}` : w.path);
    if (idx !== undefined) virt.scrollToIndex(idx, { align: "center" });
  });
  const jump = useCallback((path: string) => {
    const idx = now.current.at.get(path);
    if (idx !== undefined) virt.scrollToIndex(idx, { align: "start" });
  }, []); // eslint-disable-line react-hooks/exhaustive-deps

  const vitems = virt.getVirtualItems();
  const offset = virt.scrollOffset ?? 0;
  const topItem = vitems.find((v) => v.end > offset) ?? vitems[0];
  const topFile = topItem && items[topItem.index] ? shown[items[topItem.index].fi] : undefined;
  const treeRoot = useMemo(() => tree(shown), [shown]);
  const review: Review = { reviewed, toggle: gh ? toggleReview : undefined };
  const treeEl = useMemo(() => <Tree dir={treeRoot} jump={jump} active={topFile?.path} review={review} />, [treeRoot, jump, topFile?.path, reviewed, gh]); // eslint-disable-line react-hooks/exhaustive-deps

  const head = (f: File) => <FileHead f={f} folded={isFolded(f.path)} hits={hits.get(f.path) ?? 0} gh={gh} reviewed={reviewed.has(f.path)} onFold={toggleFold} onReview={toggleReview} onComment={commentFile} />;
  const rowProps = { q, hl, choose, extend, touch };
  const content = (it: Item) => {
    const f = shown[it.fi];
    if (it.kind === "head") return head(f);
    if (it.kind === "end") return <div className="pb-3"><div className={`h-2 rounded-b-lg border border-t-0 border-slate-200 bg-white dark:border-border dark:bg-surface`} /></div>;
    const box = "border-x border-slate-200 bg-white dark:border-border dark:bg-surface";
    if (it.kind === "form") {
      const p = pick!;
      const label = it.whole ? `Comentario sobre ${f.path}…` : `Comentario en líneas ${number(f.lines[p.start])}–${number(f.lines[p.end])}…`;
      return (
        <div className={`${box} border-y px-3`}>
          <CommentForm draft={draft} placeholder={label} suggest={!!onSuggest} steerable={steerable} rangeless={p.start < 0} pending={onSuggest ? suggest.isPending : ask.isPending} agentPending={agent.isPending} agentAvailable={!!agent.data?.available}
            onAsk={(body) => askOrLaunch(f, p, body)} onSuggest={(body, steer) => suggest.mutate({ f, p, body, steer })} onPublish={(body) => publish(f, p, body)} onCancel={() => setPick(undefined)} />
        </div>
      );
    }
    const inPick = (i: number | undefined) => i !== undefined && pick?.file === f.path && i >= pick.start && i <= pick.end;
    return (
      <div className={`${box} font-mono text-xs leading-5`}>
        {it.kind === "line"
          ? <UnifiedRow path={f.path} i={it.i} l={f.lines[it.i]} picked={inPick(it.i)} {...rowProps} />
          : <SplitRow path={f.path} lines={f.lines} r={it.r} pickedL={inPick(it.r.left)} pickedR={inPick(it.r.right)} {...rowProps} />}
      </div>
    );
  };

  // While the diff loads, the real layout renders with placeholder bars in place of files and code.
  const loading = text === undefined && diff.isPending;
  const bar = "rounded bg-slate-200 motion-safe:animate-pulse dark:bg-raised";
  const widths = [72, 55, 88, 40, 64, 80, 35, 70, 50, 60];
  const treeSkeleton = (
    <div role="status" aria-label="Cargando ficheros" className="space-y-2 px-1 pt-1">
      {widths.slice(0, 8).map((w, i) => <div key={i} className={`h-3.5 ${bar}`} style={{ width: `${w}%`, marginLeft: i % 3 ? 12 : 0 }} />)}
    </div>
  );
  const filesSkeleton = [0, 1].map((k) => (
    <section key={k} className={panel} aria-hidden="true">
      <div className="flex items-center justify-between gap-2 rounded-t-lg border-b border-slate-200 bg-slate-50 px-3 py-2 dark:border-border dark:bg-raised">
        <div className={`h-3.5 w-56 max-w-[60%] ${bar}`} />
        <div className={`h-3.5 w-16 ${bar}`} />
      </div>
      <div className="space-y-2 p-3">
        {widths.map((w, i) => <div key={i} className={`h-3 ${bar}`} style={{ width: `${(w + k * 13) % 90 + 10}%` }} />)}
      </div>
    </section>
  ));

  const toolBtn = "grid h-8 w-8 shrink-0 place-items-center rounded-md border border-slate-300 bg-white hover:bg-slate-100 dark:border-border dark:bg-surface dark:hover:bg-raised";
  const tools = (
    <>
      <button type="button" onClick={() => setPanelOpen((v) => !v)} title={panelOpen ? "Ocultar explorador de archivos" : "Mostrar explorador de archivos"} aria-label={panelOpen ? "Ocultar explorador de archivos" : "Mostrar explorador de archivos"} aria-expanded={panelOpen} className={toolBtn}>
        <TreeView aria-hidden="true" weight={panelOpen ? "fill" : "regular"} className="size-4" />
      </button>
      <button type="button" onClick={() => setSplit((v) => !v)} title={split ? "Vista unificada" : "Vista dividida"} aria-label={split ? "Vista unificada" : "Vista dividida"} aria-pressed={split} className={toolBtn}>
        {split ? <Rows aria-hidden="true" className="size-4" /> : <Columns aria-hidden="true" className="size-4" />}
      </button>
      <button type="button" onClick={() => setFull((v) => !v)} title={full ? "Salir de pantalla completa (Esc)" : "Pantalla completa"} aria-label={full ? "Salir de pantalla completa" : "Pantalla completa"} className={toolBtn}>
        {full ? <ArrowsIn weight="regular" aria-hidden="true" className="size-4" /> : <ArrowsOut weight="regular" aria-hidden="true" className="size-4" />}
      </button>
    </>
  );

  // Full screen fills the host column when the page provides one (the PR page
  // with its chat open), else the whole window.
  const host = useContext(FullscreenHost);
  const view = (
    <div className={full ? `${host ? "absolute" : "fixed h-dvh"} inset-0 z-50 overflow-hidden overscroll-none bg-slate-50 p-3 dark:bg-canvas` : fill ? "h-full min-h-0" : ""}>
    {/* A host (the thread's changes panel) can take the buttons into its own header. */}
    {toolsHost && !full && createPortal(tools, toolsHost)}
    <div className={`grid gap-3 ${panelOpen ? "md:grid-cols-[16rem_1fr]" : toolsHost && !full ? "" : fill && !full ? "md:grid-cols-[3rem_1fr]" : "md:grid-cols-[2.25rem_1fr]"} ${full || fill ? "h-full grid-rows-[auto_minmax(0,1fr)] md:grid-rows-1" : ""}`}>
      {panelOpen ? (
      <aside className={`${full || fill ? "h-[30vh] md:h-auto" : "h-[30vh] md:h-[80vh]"} ${panel} flex min-h-0 flex-col overflow-hidden`}>
          <div className="flex shrink-0 gap-1.5 p-2">
            <Input aria-label="Buscar fichero o texto" value={filter} onChange={(e) => setFilter(e.target.value)} onKeyDown={(e) => e.key === "Enter" && shown[0] && jump(shown[0].path)} placeholder="Buscar fichero o texto…" className="min-w-0 flex-1 py-1" />
            {(!toolsHost || full) && tools}
          </div>
          {!loading && gh && <div className="shrink-0 px-3 pb-1 text-xs text-slate-500 dark:text-muted">{files.filter((f) => reviewed.has(f.path)).length}/{files.length} revisados</div>}
          <div className="min-h-0 flex-1 overflow-auto overscroll-contain px-2 pb-2">{loading ? treeSkeleton : treeEl}</div>
      </aside>
      ) : (
      (!toolsHost || full) && <aside className={`flex gap-1.5 md:flex-col ${fill && !full ? "justify-center p-2 md:items-center md:justify-start md:px-0" : ""}`}>{tools}</aside>
      )}
      <div className={`flex min-h-0 min-w-0 flex-col gap-3 ${full || fill ? "" : "h-[80vh]"}`}>
        <ErrorText error={diff.error ?? comment.error ?? ask.error ?? suggest.error ?? mark.error} />
        {launching && <ReviewLaunch repo={repo} pr={pr} busy={ask.isPending} error={ask.error} onClose={() => setLaunching(undefined)} onLaunch={(launch) => ask.mutate({ ...launching, launch })} />}
        <div ref={scroller} className="min-h-0 flex-1 overflow-auto overscroll-contain">
          {loading ? <div className="space-y-3">{filesSkeleton}</div> : (
            <>
              <div className="sticky top-0 z-10 h-0">{topFile && head(topFile)}</div>
              <div className="relative w-full" style={{ height: virt.getTotalSize() }}>
                {vitems.map((v) => items[v.index] && (
                  <div key={v.key} data-index={v.index} ref={virt.measureElement} className="absolute left-0 top-0 w-full" style={{ transform: `translateY(${v.start}px)` }}>
                    {content(items[v.index])}
                  </div>
                ))}
              </div>
            </>
          )}
        </div>
      </div>
    </div>
    </div>
  );
  return full && host ? createPortal(view, host) : view;
});
