import { useEffect, useMemo, useState } from "react";
import { ArrowsIn, ArrowsOut, CaretRight, ChatCircle, CircleNotch, File as FileIcon, Folder, FolderOpen } from "@phosphor-icons/react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { api } from "../api/client";
import { Button, ErrorText } from "../components/ui";
import { highlight, languageOf, loadLanguage } from "../highlight";

type Side = "LEFT" | "RIGHT";
type Line = { text: string; kind: "add" | "del" | "ctx" | "hunk"; old?: number; new?: number };
type File = { path: string; lines: Line[]; added: number; removed: number };
type Pick = { file: string; start: number; end: number }; // start -1: whole file

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

function Tree({ dir, jump, active, depth = 0 }: { dir: Dir; jump: (path: string) => void; active?: string; depth?: number }) {
  return (
    <ul className={depth ? "ml-2 border-l border-slate-200 pl-1.5 dark:border-border" : ""}>
      {dir.dirs.map((d) => (
        <li key={d.name}>
          <details open className="group">
            <summary title={d.name} className="flex cursor-pointer list-none items-center gap-1 rounded px-1 py-0.5 text-xs font-medium hover:bg-slate-100 dark:hover:bg-raised [&::-webkit-details-marker]:hidden">
              <CaretRight aria-hidden="true" className="size-3 shrink-0 text-slate-400 transition-transform group-open:rotate-90" />
              <Folder aria-hidden="true" className="size-4 shrink-0 text-slate-400 group-open:hidden" />
              <FolderOpen aria-hidden="true" className="hidden size-4 shrink-0 text-slate-400 group-open:block" />
              <span className="truncate">{d.name}</span>
            </summary>
            <Tree dir={d} jump={jump} active={active} depth={depth + 1} />
          </details>
        </li>
      ))}
      {dir.files.map((f) => (
        <li key={f.path}>
          <button type="button" title={f.path} onClick={() => jump(f.path)} className={`flex w-full items-center gap-1 rounded px-1 py-0.5 text-left text-xs hover:bg-slate-100 dark:hover:bg-raised ${active === f.path ? "bg-blue-50 font-medium text-blue-700 dark:bg-raised dark:text-accent-text" : ""}`}>
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

export function Diff({ repo, pr }: { repo: string; pr: number }) {
  const qc = useQueryClient();
  const diff = useQuery({ queryKey: ["github-diff", repo, pr], queryFn: () => api.githubDiff(repo, pr) });
  const files = useMemo(() => parseDiff(diff.data?.diff ?? ""), [diff.data]);
  const [filter, setFilter] = useState("");
  const [pick, setPick] = useState<Pick>();
  const [body, setBody] = useState("");
  const [drag, setDrag] = useState<{ file: string; from: number }>();
  const [open, setOpen] = useState(false);
  const [active, setActive] = useState<string>();
  const [full, setFull] = useState(false);
  const [loaded, setLoaded] = useState(0); // bumps when a language finishes loading
  useEffect(() => {
    if (!full) return;
    const esc = (e: KeyboardEvent) => e.key === "Escape" && setFull(false);
    window.addEventListener("keydown", esc);
    return () => window.removeEventListener("keydown", esc);
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
  const comment = useMutation({
    mutationFn: (a: Parameters<typeof api.githubAct>[3]) => api.githubAct(repo, "pulls", pr, a),
    onSuccess: () => {
      setPick(undefined); setBody("");
      qc.invalidateQueries({ queryKey: ["github-detail", repo, "pulls", pr] });
    },
  });
  const shown = files.filter((f) => f.path.toLowerCase().includes(filter.toLowerCase()));

  const choose = (file: string, i: number, shift: boolean) => {
    const from = shift && pick?.file === file && pick.start >= 0 ? pick.start : i;
    setOpen(false);
    setDrag({ file, from });
    setPick({ file, start: Math.min(from, i), end: Math.max(from, i) });
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
                        <textarea aria-label="Comentario de revisión" autoFocus value={body} onChange={(e) => setBody(e.target.value)} rows={3} placeholder={placeholder} className="w-full rounded border border-slate-300 bg-white p-2 text-sm dark:border-border dark:bg-surface" />
                        <div className="flex gap-2">
                          <Button disabled={comment.isPending || !body.trim()} onClick={() => publish(f, p)}>Publicar en GitHub</Button>
                          <Button variant="ghost" onClick={() => setPick(undefined)}>Cancelar</Button>
                        </div>
                      </div>
  );
  const jump = (path: string) => {
    setActive(path);
    document.getElementById(anchor(path))?.closest("section")?.scrollIntoView({ behavior: "smooth", block: "start" });
  };
  const treeRoot = useMemo(() => tree(shown), [filter, files]); // eslint-disable-line react-hooks/exhaustive-deps

  if (diff.isPending) {
    return (
      <div className={`${panel} flex items-center justify-center gap-2 p-10 text-sm text-slate-500 dark:text-muted`} role="status">
        <CircleNotch aria-hidden="true" className="size-5 animate-spin" /> Cargando diff…
      </div>
    );
  }

  return (
    <div className={full ? "fixed inset-0 z-50 overflow-hidden bg-slate-50 p-3 dark:bg-canvas" : ""}>
    <div className={`grid gap-3 md:grid-cols-[16rem_1fr] ${full ? "h-full grid-rows-[auto_minmax(0,1fr)] md:grid-rows-1" : ""}`}>
      <aside className={`${full ? "h-[30vh] md:h-auto" : "h-[30vh] md:h-[80vh]"} overflow-auto`}>
        <div className={`${panel} min-h-full p-2`}>
          <div className="mb-2 flex gap-1.5">
            <input aria-label="Buscar fichero" value={filter} onChange={(e) => setFilter(e.target.value)} onKeyDown={(e) => e.key === "Enter" && shown[0] && jump(shown[0].path)} placeholder="Buscar fichero…" className="min-w-0 flex-1 rounded-md border border-slate-300 bg-white px-2 py-1 text-sm dark:border-border dark:bg-surface" />
            <button type="button" onClick={() => setFull((v) => !v)} title={full ? "Salir de pantalla completa (Esc)" : "Pantalla completa"} aria-label={full ? "Salir de pantalla completa" : "Pantalla completa"} className="grid w-8 shrink-0 place-items-center rounded-md border border-slate-300 bg-white hover:bg-slate-100 dark:border-border dark:bg-surface dark:hover:bg-raised">
              {full ? <ArrowsIn aria-hidden="true" className="size-4" /> : <ArrowsOut aria-hidden="true" className="size-4" />}
            </button>
          </div>
          <Tree dir={treeRoot} jump={jump} active={active} />
        </div>
      </aside>
      <div className={`min-w-0 space-y-3 overflow-auto overscroll-contain ${full ? "" : "md:h-[80vh]"}`}>
        <ErrorText error={diff.error ?? comment.error} />
        {shown.map((f) => (
          <section key={f.path} className={panel}>
            <div id={anchor(f.path)} className="sticky top-0 z-10 flex scroll-mt-0 items-center justify-between gap-2 rounded-t-lg border-b border-slate-200 bg-slate-50 px-3 py-1.5 dark:border-border dark:bg-raised">
              <span className="min-w-0 truncate font-mono text-xs font-semibold" title={f.path}>{f.path}</span>
              <span className="flex shrink-0 items-center gap-2 font-mono text-xs">
                <span className="text-green-700 dark:text-green-400">+{f.added}</span>
                <span className="text-red-700 dark:text-red-400">−{f.removed}</span>
                <button type="button" title="Comentar el fichero" aria-label="Comentar el fichero" onClick={() => { setPick({ file: f.path, start: -1, end: -1 }); setOpen(true); }} className="rounded p-0.5 text-slate-400 hover:bg-slate-200 hover:text-blue-600 dark:hover:bg-border dark:hover:text-accent-text">
                  <ChatCircle aria-hidden="true" className="size-4" />
                </button>
              </span>
            </div>
            {pick?.file === f.path && pick.start < 0 && <div className="px-3">{form(f, pick, `Comentario sobre ${f.path}…`)}</div>}
            <pre className="overflow-x-auto font-mono text-xs leading-5">
              {f.lines.map((l, i) => {
                const picked = pick?.file === f.path && i >= pick.start && i <= pick.end;
                return (
                  <div key={i}>
                    <div onMouseEnter={() => l.kind !== "hunk" && extend(f.path, i)} className={`flex ${picked ? "bg-yellow-100 dark:bg-yellow-900" : color[l.kind]}`}>
                      {l.kind === "hunk" ? null : (
                        <button type="button" title="Comentar esta línea" onMouseDown={(e) => { e.preventDefault(); choose(f.path, i, e.shiftKey); }} onMouseEnter={() => extend(f.path, i)} className={`w-20 shrink-0 select-none border-r border-slate-200 pr-2 text-right text-slate-400 hover:text-blue-600 dark:border-border dark:hover:text-accent-text ${picked ? "" : gutter[l.kind]}`}>
                          <span className="inline-block w-8">{l.old ?? ""}</span> <span className="inline-block w-8">{l.new ?? ""}</span>
                        </button>
                      )}
                      {html.get(f.path)?.[i] !== undefined ? (
                        <span className="min-w-0 flex-1 px-2"><span>{l.text[0]}</span><span dangerouslySetInnerHTML={{ __html: html.get(f.path)![i]! }} /></span>
                      ) : (
                        <span className={`min-w-0 flex-1 px-2 ${l.kind === "hunk" ? "pl-3" : ""}`}>{l.text || " "}</span>
                      )}
                    </div>
                    {open && !drag && pick?.file === f.path && pick.end === i && (
                      <div className="px-3">{form(f, pick, `Comentario en líneas ${number(f.lines[pick.start])}–${number(l)}…`)}</div>
                    )}
                  </div>
                );
              })}
            </pre>
          </section>
        ))}
      </div>
    </div>
    </div>
  );
}
