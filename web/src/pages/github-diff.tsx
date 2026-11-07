import { useEffect, useMemo, useRef, useState } from "react";
import { ArrowsIn, CaretDown, ArrowsOut, CaretRight, Columns, Rows, SidebarSimple, ChatCircle, File as FileIcon, Folder, FolderOpen } from "@phosphor-icons/react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { api } from "../api/client";
import { Button, ErrorText, Input, Textarea, keepAboveKeyboard } from "../components/ui";
import { ThreadLink } from "../components/thread-link";
import { highlight, languageOf, loadLanguage } from "../highlight";

type Side = "LEFT" | "RIGHT";
type Line = { text: string; kind: "add" | "del" | "ctx" | "hunk"; old?: number; new?: number };
type File = { path: string; lines: Line[]; added: number; removed: number };
type Pick = { file: string; start: number; end: number }; // start -1: whole file

// patchHash fingerprints a file patch so a review mark expires when the file changes.
async function patchHash(f: File): Promise<string> {
  const data = new TextEncoder().encode(f.lines.map((l) => l.text).join("\n"));
  return Array.from(new Uint8Array(await crypto.subtle.digest("SHA-1", data)), (b) => b.toString(16).padStart(2, "0")).join("");
}

// parseDiff splits a unified diff into files with old and new line numbers.
function parseDiff(diff: string): File[] {
  const files: File[] = [];
  let f: File | undefined;
  let o = 0, n = 0;
  for (const text of diff.split("\n")) {
    if (text.startsWith("diff --git ")) {
      f = { path: text.replace(/^diff --git a\/.* b\//, ""), lines: [], added: 0, removed: 0 };
      files.push(f);
      continue;
    }
    if (!f) continue;
    const h = /^@@ -(\d+)(?:,\d+)? \+(\d+)/.exec(text);
    if (h) {
      o = Number(h[1]); n = Number(h[2]);
      f.lines.push({ text, kind: "hunk" });
    } else if (!f.lines.length) {
      continue; // file header: index, ---, +++
    } else if (text.startsWith("+")) {
      f.lines.push({ text, kind: "add", new: n++ }); f.added++;
    } else if (text.startsWith("-")) {
      f.lines.push({ text, kind: "del", old: o++ }); f.removed++;
    } else if (text.startsWith(" ")) {
      f.lines.push({ text, kind: "ctx", old: o++, new: n++ });
    }
  }
  return files;
}

type Row = { hunk?: number; left?: number; right?: number };

// splitRows pairs removed and added lines side by side, like GitHub's split view.
function splitRows(lines: Line[]): Row[] {
  const rows: Row[] = [];
  let dels: number[] = [], adds: number[] = [];
  const flush = () => {
    for (let k = 0; k < Math.max(dels.length, adds.length); k++) rows.push({ left: dels[k], right: adds[k] });
    dels = []; adds = [];
  };
  lines.forEach((l, i) => {
    if (l.kind === "del") { if (adds.length) flush(); dels.push(i); }
    else if (l.kind === "add") adds.push(i);
    else { flush(); rows.push(l.kind === "hunk" ? { hunk: i } : { left: i, right: i }); }
  });
  flush();
  return rows;
}

type Dir = { name: string; dirs: Dir[]; files: File[] };

// tree groups files by folder and joins folders that hold a single folder.
function tree(files: File[]): Dir {
  const root: Dir = { name: "", dirs: [], files: [] };
  for (const f of files) {
    let d = root;
    for (const part of f.path.split("/").slice(0, -1)) {
      let next = d.dirs.find((x) => x.name === part);
      if (!next) d.dirs.push((next = { name: part, dirs: [], files: [] }));
      d = next;
    }
    d.files.push(f);
  }
  const squash = (d: Dir): Dir => {
    d.dirs = d.dirs.map(squash);
    if (d.name && d.dirs.length === 1 && !d.files.length) return { ...d.dirs[0], name: `${d.name}/${d.dirs[0].name}` };
    return d;
  };
  return squash(root);
}

type Review = { reviewed: Set<string>; toggle?: (path: string) => void };

function Tree({ dir, jump, active, review, depth = 0 }: { dir: Dir; jump: (path: string) => void; active?: string; review: Review; depth?: number }) {
  return (
    <ul className={depth ? "ml-2 border-l border-slate-200 pl-1.5 dark:border-border" : ""}>
      {dir.dirs.map((d) => (
        <li key={d.name}>
          <details open className="group">
            <summary title={d.name} className="flex cursor-pointer list-none items-center gap-1 rounded px-1 py-0.5 text-xs font-medium hover:bg-slate-100 dark:hover:bg-raised [&::-webkit-details-marker]:hidden">
              <CaretRight weight="regular" aria-hidden="true" className="size-3 shrink-0 text-slate-400 transition-transform group-open:rotate-90" />
              <Folder aria-hidden="true" className="size-4 shrink-0 text-slate-400 group-open:hidden" />
              <FolderOpen aria-hidden="true" className="hidden size-4 shrink-0 text-slate-400 group-open:block" />
              <span className="truncate">{d.name}</span>
            </summary>
            <Tree dir={d} jump={jump} active={active} review={review} depth={depth + 1} />
          </details>
        </li>
      ))}
      {dir.files.map((f) => (
        <li key={f.path} className="flex items-center gap-1">
          {review.toggle && <input type="checkbox" aria-label={`Revisado: ${f.path}`} title="Revisado" checked={review.reviewed.has(f.path)} onChange={() => review.toggle!(f.path)} className="ml-1 shrink-0" />}
          <button type="button" title={f.path} onClick={() => jump(f.path)} className={`flex min-w-0 flex-1 items-center gap-1 rounded px-1 py-0.5 text-left text-xs hover:bg-slate-100 dark:hover:bg-raised ${review.reviewed.has(f.path) ? "text-slate-400" : ""} ${active === f.path ? "bg-blue-50 font-medium text-blue-700 dark:bg-raised dark:text-accent-text" : ""}`}>
            <FileIcon aria-hidden="true" className="size-4 shrink-0 text-slate-400" />
            <span className="min-w-0 flex-1 truncate">{f.path.split("/").pop()}</span>
            <span className="shrink-0 font-mono text-[11px] text-green-700 dark:text-green-400">+{f.added}</span>
            <span className="shrink-0 font-mono text-[11px] text-red-700 dark:text-red-400">−{f.removed}</span>
          </button>
        </li>
      ))}
    </ul>
  );
}

const side = (l: Line): Side => (l.kind === "del" ? "LEFT" : "RIGHT");
const number = (l: Line) => (l.kind === "del" ? l.old! : l.new!);
const color = {
  add: "bg-green-50 dark:bg-green-950/60",
  del: "bg-red-50 dark:bg-red-950/60",
  hunk: "border-y border-blue-100 bg-blue-50 text-blue-700 dark:border-border dark:bg-raised dark:text-muted",
  ctx: "",
};
const gutter = { add: "bg-green-100 dark:bg-green-900/40", del: "bg-red-100 dark:bg-red-900/40", hunk: "", ctx: "bg-slate-50 dark:bg-raised" };
const panel = "rounded-lg border border-slate-200 bg-white dark:border-border dark:bg-surface";
const anchor = (path: string) => `diff-${path}`;

const markClass = "rounded-sm bg-amber-300 text-slate-900 dark:bg-amber-500/70 dark:text-white";
const escRe = (t: string) => t.replace(/[.*+?^${}()|[\]\\]/g, "\\$&");
const escHtml = (t: string) => t.replace(/&/g, "&amp;").replace(/</g, "&lt;").replace(/>/g, "&gt;").replace(/"/g, "&quot;").replace(/'/g, "&#39;");

// markHtml wraps the search term inside highlighted code, touching only the text between tags.
function markHtml(html: string, q: string) {
  if (!q) return html;
  const re = new RegExp(`(${escRe(escHtml(q))})`, "gi");
  return html.split(/(<[^>]+>)/).map((part) => (part.startsWith("<") ? part : part.replace(re, `<mark class="${markClass}">$1</mark>`))).join("");
}

// markText does the same for plain lines.
function markText(text: string, q: string) {
  if (!q) return text;
  return text.split(new RegExp(`(${escRe(q)})`, "gi")).map((part, i) => (i % 2 ? <mark key={i} className={markClass}>{part}</mark> : part));
}

// Suggestion is a selected range sent to a chat instead of GitHub.
export type Suggestion = { path: string; start: number; end: number; snippet: string; question: string; steer: boolean };

// Diff shows a pull request diff, or the given diff text when onSuggest sends
// selections to a chat; GitHub reviews and comments only apply to the first.
// fill makes the diff take its container's height instead of 80vh.
export function Diff({ repo = "", pr = 0, text, onSuggest, fill = false }: { repo?: string; pr?: number; text?: string; onSuggest?: (s: Suggestion) => Promise<void>; fill?: boolean }) {
  const qc = useQueryClient();
  const gh = !onSuggest && !!repo && !!pr;
  const diff = useQuery({ queryKey: ["github-diff", repo, pr], queryFn: () => api.githubDiff(repo, pr), enabled: text === undefined });
  const source = text ?? diff.data?.diff ?? "";
  const files = useMemo(() => parseDiff(source), [source]);
  const [filter, setFilter] = useState("");
  const [pick, setPick] = useState<Pick>();
  const [body, setBody] = useState("");
  const [drag, setDrag] = useState<{ file: string; from: number }>();
  const [open, setOpen] = useState(false);
  const [active, setActive] = useState<string>();
  const [full, setFull] = useState(false);
  // The last chosen layout is remembered in this browser for every diff.
  const [panelOpen, setPanelOpen] = useState(true);
  const [split, setSplitState] = useState(() => { try { return localStorage.getItem("dory.diff-view") === "split"; } catch { return false; } });
  const setSplit = (f: (v: boolean) => boolean) => setSplitState((v) => {
    const next = f(v);
    try { localStorage.setItem("dory.diff-view", next ? "split" : "unified"); } catch { /* storage unavailable */ }
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
  const html = useMemo(() => {
    const out = new Map<string, (string | undefined)[]>();
    for (const f of files) {
      const lang = languageOf(f.path);
      if (lang) out.set(f.path, f.lines.map((l) => (l.kind === "hunk" ? undefined : highlight(l.text.slice(1), lang))));
    }
    return out;
  }, [files, loaded]); // eslint-disable-line react-hooks/exhaustive-deps
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
  const reviews = useQuery({ queryKey: ["pull-reviews", repo, pr], queryFn: () => api.pullReviews(repo, pr), enabled: gh });
  const reviewed = useMemo(() => new Set((reviews.data ?? []).filter((v) => v.patch_hash === hashes.get(v.path)).map((v) => v.path)), [reviews.data, hashes]);
  const mark = useMutation({
    mutationFn: ({ path, on }: { path: string; on: boolean }) => (on ? api.markPullReview(repo, pr, path, hashes.get(path)!) : api.unmarkPullReview(repo, pr, path)),
    onSettled: () => qc.invalidateQueries({ queryKey: ["pull-reviews", repo, pr] }),
  });
  // Reviewed files fold unless the user opens them.
  const [folded, setFolded] = useState(new Map<string, boolean>());
  const isFolded = (path: string) => folded.get(path) ?? reviewed.has(path);
  const toggleReview = (path: string) => {
    if (!hashes.has(path)) return;
    const on = !reviewed.has(path);
    setFolded((m) => { const n = new Map(m); n.delete(path); return n; });
    mark.mutate({ path, on });
  };
  const review: Review = { reviewed, toggle: gh ? toggleReview : undefined };
  const agent = useQuery({ queryKey: ["pull-agent", repo, pr], queryFn: () => api.pullAgent(repo, pr), enabled: gh });
  const ask = useMutation({
    mutationFn: ({ f, p }: { f: File; p: Pick }) => {
      const lines = f.lines.slice(p.start, p.end + 1).filter((l) => l.kind !== "hunk");
      const nums = lines.map(number);
      return api.askPullAgent(repo, pr, { path: f.path, start: Math.min(...nums), end: Math.max(...nums), snippet: lines.map((l) => l.text).join("\n"), question: body.trim() });
    },
    onSuccess: () => { setPick(undefined); setBody(""); },
  });
  const suggest = useMutation({
    mutationFn: ({ f, p, steer }: { f: File; p: Pick; steer: boolean }) => {
      const lines = f.lines.slice(p.start, p.end + 1).filter((l) => l.kind !== "hunk");
      const nums = lines.map(number);
      return onSuggest!({ path: f.path, start: Math.min(...nums), end: Math.max(...nums), snippet: lines.map((l) => l.text).join("\n"), question: body.trim(), steer });
    },
    onSuccess: () => { setPick(undefined); setBody(""); },
  });
  const comment = useMutation({
    mutationFn: (a: Parameters<typeof api.githubAct>[3]) => api.githubAct(repo, "pulls", pr, a),
    onSuccess: () => {
      setPick(undefined); setBody("");
      qc.invalidateQueries({ queryKey: ["github-detail", repo, "pulls", pr] });
    },
  });
  // The search matches file paths and the text of the changed lines.
  const q = filter.trim().toLowerCase();
  const hit = (l: Line) => !!q && l.kind !== "hunk" && l.text.slice(1).toLowerCase().includes(q);
  const hits = useMemo(() => new Map(files.map((f) => [f.path, q ? f.lines.filter(hit).length : 0])), [files, q]); // eslint-disable-line react-hooks/exhaustive-deps
  const shown = files.filter((f) => !q || f.path.toLowerCase().includes(q) || hits.get(f.path)! > 0);

  // On touch screens a drag scrolls; tapping a second line number extends the selection instead.
  const touch = useRef(false);
  const choose = (file: string, i: number, shift: boolean) => {
    const from = shift && pick?.file === file && pick.start >= 0 ? pick.start : i;
    setPick({ file, start: Math.min(from, i), end: Math.max(from, i) });
    // A tap has no drag: the comment box moves when the range changes, and a drag
    // would keep extending to whatever line slides under the finger.
    if (touch.current) { setDrag(undefined); setOpen(true); return; }
    setOpen(false);
    setDrag({ file, from });
  };
  const extend = (file: string, i: number) => {
    if (drag?.file === file) setPick({ file, start: Math.min(drag.from, i), end: Math.max(drag.from, i) });
  };
  const publish = (f: File, p: Pick) => {
    if (p.start < 0) return comment.mutate({ type: "comment_file", body, path: f.path });
    const first = f.lines[p.start], last = f.lines[p.end];
    comment.mutate({ type: "comment_lines", body, path: f.path, line: number(last), side: side(last), start_line: number(first), start_side: side(first) });
  };
  const form = (f: File, p: Pick, placeholder: string) => (
    <div className="my-2 whitespace-normal font-sans">
                        <Textarea aria-label="Comentario de revisión" autoFocus onFocus={(e) => keepAboveKeyboard(e.currentTarget.parentElement!)} value={body} onChange={(e) => setBody(e.target.value)} rows={3} placeholder={placeholder} className="p-2" />
                        <div className="mt-2 flex gap-2">
                          {onSuggest ? (
                            <>
                              <Button disabled={suggest.isPending || !body.trim() || p.start < 0} onClick={() => suggest.mutate({ f, p, steer: false })} title="Envía la sugerencia al hilo; si está trabajando, se encola">Sugerir al chat</Button>
                              <Button variant="ghost" disabled={suggest.isPending || !body.trim() || p.start < 0} onClick={() => suggest.mutate({ f, p, steer: true })} title="Inyecta la sugerencia en el turno en curso">Steer</Button>
                            </>
                          ) : <>
                          <Button disabled={comment.isPending || !body.trim()} onClick={() => publish(f, p)}>Publicar en GitHub</Button>
                          {p.start >= 0 && (
                            <Button disabled={ask.isPending || !body.trim() || !agent.data?.available} onClick={() => ask.mutate({ f, p })}
                              title={agent.data?.available ? "Consulta privada al hilo donde se implementó la PR; no se publica en GitHub" : "Esta PR no tiene un item de Dory con hilo de implementación"}>Preguntar al agente</Button>
                          )}
                          </>}
                          <Button variant="ghost" onClick={() => setPick(undefined)}>Cancelar</Button>
                        </div>
                      </div>
  );
  const jump = (path: string) => {
    setActive(path);
    document.getElementById(anchor(path))?.closest("section")?.scrollIntoView({ behavior: "smooth", block: "start" });
  };
  const treeRoot = useMemo(() => tree(shown), [filter, files]); // eslint-disable-line react-hooks/exhaustive-deps

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
      <button type="button" onClick={() => setPanelOpen((v) => !v)} title={panelOpen ? "Ocultar panel" : "Mostrar panel"} aria-label={panelOpen ? "Ocultar panel" : "Mostrar panel"} aria-expanded={panelOpen} className={toolBtn}>
        {panelOpen ? <SidebarSimple aria-hidden="true" weight="fill" className="size-4" /> : <SidebarSimple aria-hidden="true" className="size-4" />}
      </button>
      <button type="button" onClick={() => setSplit((v) => !v)} title={split ? "Vista unificada" : "Vista dividida"} aria-label={split ? "Vista unificada" : "Vista dividida"} aria-pressed={split} className={toolBtn}>
        {split ? <Rows aria-hidden="true" className="size-4" /> : <Columns aria-hidden="true" className="size-4" />}
      </button>
      <button type="button" onClick={() => setFull((v) => !v)} title={full ? "Salir de pantalla completa (Esc)" : "Pantalla completa"} aria-label={full ? "Salir de pantalla completa" : "Pantalla completa"} className={toolBtn}>
        {full ? <ArrowsIn weight="regular" aria-hidden="true" className="size-4" /> : <ArrowsOut weight="regular" aria-hidden="true" className="size-4" />}
      </button>
    </>
  );

  return (
    <div className={full ? "fixed inset-0 z-50 h-dvh overflow-hidden overscroll-none bg-slate-50 p-3 dark:bg-canvas" : fill ? "h-full min-h-0" : ""}>
    <div className={`grid gap-3 ${panelOpen ? "md:grid-cols-[16rem_1fr]" : "md:grid-cols-[2.25rem_1fr]"} ${full || fill ? "h-full grid-rows-[auto_minmax(0,1fr)] md:grid-rows-1" : ""}`}>
      {panelOpen ? (
      <aside className={`${full || fill ? "h-[30vh] md:h-auto" : "h-[30vh] md:h-[80vh]"} ${panel} flex min-h-0 flex-col overflow-hidden`}>
          <div className="flex shrink-0 gap-1.5 p-2">
            <Input aria-label="Buscar fichero o texto" value={filter} onChange={(e) => setFilter(e.target.value)} onKeyDown={(e) => e.key === "Enter" && shown[0] && jump(shown[0].path)} placeholder="Buscar fichero o texto…" className="min-w-0 flex-1 py-1" />
            {tools}
          </div>
          {!loading && gh && <div className="shrink-0 px-3 pb-1 text-xs text-slate-500 dark:text-muted">{files.filter((f) => reviewed.has(f.path)).length}/{files.length} revisados</div>}
          <div className="min-h-0 flex-1 overflow-auto overscroll-contain px-2 pb-2">{loading ? treeSkeleton : <Tree dir={treeRoot} jump={jump} active={active} review={review} />}</div>
      </aside>
      ) : (
      <aside className="flex gap-1.5 md:flex-col">{tools}</aside>
      )}
      <div className={`min-w-0 space-y-3 overflow-auto overscroll-contain ${full || fill ? "" : "md:h-[80vh]"}`}>
        <ErrorText error={diff.error ?? comment.error ?? ask.error ?? mark.error ?? suggest.error} />
        {ask.data && (
          <div role="status" className="flex items-center justify-between gap-2 rounded-lg border border-blue-200 bg-blue-50 px-3 py-2 text-sm dark:border-border dark:bg-raised">
            <span>Consulta enviada al hilo de implementación. {ask.data.url && <ThreadLink url={ask.data.url} environmentId={ask.data.t3_environment_id} threadId={ask.data.thread_id} className="font-medium text-blue-700 underline dark:text-accent-text">Abrir hilo</ThreadLink>}</span>
            <button type="button" onClick={() => ask.reset()} aria-label="Cerrar aviso" className="text-slate-500">×</button>
          </div>
        )}
        {loading && filesSkeleton}
        {shown.map((f) => (
          <section key={f.path} className={panel}>
            <div id={anchor(f.path)} className="sticky top-0 z-10 flex scroll-mt-0 items-center justify-between gap-2 rounded-t-lg border-b border-slate-200 bg-slate-50 px-3 py-1.5 dark:border-border dark:bg-raised">
              <span className="flex min-w-0 items-center gap-1.5">
                <button type="button" onClick={() => setFolded((m) => new Map(m).set(f.path, !isFolded(f.path)))} aria-label={isFolded(f.path) ? "Desplegar fichero" : "Plegar fichero"} aria-expanded={!isFolded(f.path)} className="shrink-0 rounded text-slate-400 hover:text-blue-600">
                  {isFolded(f.path) ? <CaretRight weight="regular" aria-hidden="true" className="size-3.5" /> : <CaretDown weight="regular" aria-hidden="true" className="size-3.5" />}
                </button>
                <span className="min-w-0 truncate font-mono text-xs font-semibold" title={f.path}>{f.path}</span>
              </span>
              <span className="flex shrink-0 items-center gap-2 font-mono text-xs">
                {hits.get(f.path)! > 0 && <span className="rounded bg-amber-100 px-1.5 text-amber-800 dark:bg-amber-900/50 dark:text-amber-300">{hits.get(f.path)} coincidencia{hits.get(f.path) === 1 ? "" : "s"}</span>}
                <span className="text-green-700 dark:text-green-400">+{f.added}</span>
                <span className="text-red-700 dark:text-red-400">−{f.removed}</span>
                {gh && <label className="flex cursor-pointer items-center gap-1 font-sans text-slate-500 dark:text-muted">
                  <input type="checkbox" checked={reviewed.has(f.path)} onChange={() => toggleReview(f.path)} />Revisado
                </label>}
                {gh && <button type="button" title="Comentar el fichero" aria-label="Comentar el fichero" onClick={() => { setPick({ file: f.path, start: -1, end: -1 }); setOpen(true); }} className="rounded p-0.5 text-slate-400 hover:bg-slate-200 hover:text-blue-600 dark:hover:bg-border dark:hover:text-accent-text">
                  <ChatCircle aria-hidden="true" className="size-4" />
                </button>}
              </span>
            </div>
            {pick?.file === f.path && pick.start < 0 && <div className="px-3">{form(f, pick, `Comentario sobre ${f.path}…`)}</div>}
            {isFolded(f.path) ? null : split ? (
              <div className="overflow-x-auto font-mono text-xs leading-5">
                {splitRows(f.lines).map((r, k) => {
                  if (r.hunk !== undefined) return <div key={k} className={`whitespace-pre py-0 pl-3 ${color.hunk}`}>{f.lines[r.hunk].text}</div>;
                  const cell = (i: number | undefined, at: "old" | "new") => {
                    if (i === undefined) return <div className="flex min-w-0 bg-slate-50 dark:bg-raised/50"><span className="w-10 shrink-0 border-r border-slate-200 dark:border-border" /></div>;
                    const l = f.lines[i];
                    const picked = pick?.file === f.path && i >= pick.start && i <= pick.end;
                    const kind = l.kind === "ctx" ? "ctx" : l.kind;
                    return (
                      <div onMouseEnter={() => extend(f.path, i)} className={`flex min-w-0 ${picked ? "bg-yellow-100 dark:bg-yellow-900" : color[kind]} ${hit(l) ? "ring-1 ring-inset ring-amber-400/70" : ""}`}>
                        <button type="button" title="Comentar esta línea (en móvil, toca otra para ampliar el rango)" onPointerDown={(e) => { touch.current = e.pointerType !== "mouse"; }} onMouseDown={(e) => { e.preventDefault(); choose(f.path, i, e.shiftKey || touch.current); }} className={`w-10 shrink-0 touch-manipulation select-none border-r border-slate-200 pr-2 text-right text-slate-400 hover:text-blue-600 dark:border-border dark:hover:text-accent-text ${picked ? "" : gutter[kind]}`}>{l[at] ?? ""}</button>
                        {html.get(f.path)?.[i] !== undefined
                          ? <span className="min-w-0 flex-1 whitespace-pre-wrap break-all px-2"><span>{l.text[0]}</span><span dangerouslySetInnerHTML={{ __html: markHtml(html.get(f.path)![i]!, q) }} /></span>
                          : <span className="min-w-0 flex-1 whitespace-pre-wrap break-all px-2">{l.text ? <>{l.text[0]}{markText(l.text.slice(1), q)}</> : " "}</span>}
                      </div>
                    );
                  };
                  const ends = open && !drag && pick?.file === f.path && pick.start >= 0 && (r.left === pick.end || r.right === pick.end);
                  return (
                    <div key={k}>
                      <div className="grid grid-cols-[minmax(0,1fr)_minmax(0,1fr)] divide-x divide-slate-200 dark:divide-border">
                        {cell(r.left, "old")}
                        {cell(r.right, "new")}
                      </div>
                      {ends && <div className="px-3">{form(f, pick!, `Comentario en líneas ${number(f.lines[pick!.start])}–${number(f.lines[pick!.end])}…`)}</div>}
                    </div>
                  );
                })}
              </div>
            ) : (
            <pre className="overflow-x-auto font-mono text-xs leading-5">
              {f.lines.map((l, i) => {
                const picked = pick?.file === f.path && i >= pick.start && i <= pick.end;
                return (
                  <div key={i}>
                    <div onMouseEnter={() => l.kind !== "hunk" && extend(f.path, i)} className={`flex ${picked ? "bg-yellow-100 dark:bg-yellow-900" : color[l.kind]} ${hit(l) ? "ring-1 ring-inset ring-amber-400/70" : ""}`}>
                      {l.kind === "hunk" ? null : (
                        <button type="button" title="Comentar esta línea (en móvil, toca otra para ampliar el rango)" onPointerDown={(e) => { touch.current = e.pointerType !== "mouse"; }} onMouseDown={(e) => { e.preventDefault(); choose(f.path, i, e.shiftKey || touch.current); }} onMouseEnter={() => extend(f.path, i)} className={`w-20 shrink-0 touch-manipulation select-none border-r border-slate-200 pr-2 text-right text-slate-400 hover:text-blue-600 dark:border-border dark:hover:text-accent-text ${picked ? "" : gutter[l.kind]}`}>
                          <span className="inline-block w-8">{l.old ?? ""}</span> <span className="inline-block w-8">{l.new ?? ""}</span>
                        </button>
                      )}
                      {html.get(f.path)?.[i] !== undefined ? (
                        <span className="min-w-0 flex-1 whitespace-pre-wrap break-all px-2"><span>{l.text[0]}</span><span dangerouslySetInnerHTML={{ __html: markHtml(html.get(f.path)![i]!, q) }} /></span>
                      ) : (
                        <span className={`min-w-0 flex-1 whitespace-pre-wrap break-all px-2 ${l.kind === "hunk" ? "pl-3" : ""}`}>{l.kind === "hunk" || !l.text ? l.text || " " : <>{l.text[0]}{markText(l.text.slice(1), q)}</>}</span>
                      )}
                    </div>
                    {open && !drag && pick?.file === f.path && pick.end === i && (
                      <div className="px-3">{form(f, pick, `Comentario en líneas ${number(f.lines[pick.start])}–${number(l)}…`)}</div>
                    )}
                  </div>
                );
              })}
            </pre>
            )}
          </section>
        ))}
      </div>
    </div>
    </div>
  );
}
