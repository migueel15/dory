import { useEffect, useMemo, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { api } from "../api/client";
import { Button, Card, ErrorText } from "../components/ui";

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

function Tree({ dir, jump }: { dir: Dir; jump: (path: string) => void }) {
  return (
    <ul className="space-y-1 pl-3 text-xs">
      {dir.dirs.map((d) => (
        <li key={d.name}>
          <details open>
            <summary className="cursor-pointer break-all font-semibold">📁 {d.name}</summary>
            <Tree dir={d} jump={jump} />
          </details>
        </li>
      ))}
      {dir.files.map((f) => (
        <li key={f.path}>
          <button type="button" title={f.path} onClick={() => jump(f.path)} className="w-full break-all text-left hover:underline">
            {f.path.split("/").pop()} <span className="text-green-700 dark:text-green-400">+{f.added}</span> <span className="text-red-700 dark:text-red-400">−{f.removed}</span>
          </button>
        </li>
      ))}
    </ul>
  );
}

const side = (l: Line): Side => (l.kind === "del" ? "LEFT" : "RIGHT");
const number = (l: Line) => (l.kind === "del" ? l.old! : l.new!);
const color = { add: "bg-green-100 dark:bg-green-950", del: "bg-red-100 dark:bg-red-950", hunk: "text-slate-500 dark:text-muted", ctx: "" };
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
  const jump = (path: string) => document.getElementById(anchor(path))?.scrollIntoView({ behavior: "smooth", block: "start" });

  return (
    <div className="grid items-start gap-3 md:grid-cols-[16rem_1fr]">
      <aside className="md:max-h-[80vh] md:overflow-auto">
        <Card>
          <input aria-label="Buscar fichero" value={filter} onChange={(e) => setFilter(e.target.value)} onKeyDown={(e) => e.key === "Enter" && shown[0] && jump(shown[0].path)} placeholder="Buscar fichero…" className="mb-2 w-full rounded border border-slate-300 bg-white px-2 py-1 text-sm dark:border-border dark:bg-surface" />
          <div className="-ml-3"><Tree dir={tree(shown)} jump={jump} /></div>
        </Card>
      </aside>
      <div className="min-w-0 space-y-3 md:max-h-[80vh] md:overflow-auto md:overscroll-contain">
        <ErrorText error={diff.error ?? comment.error} />
        {shown.map((f) => (
          <Card key={f.path}>
            <div id={anchor(f.path)} className="mb-1 flex scroll-mt-2 items-start justify-between gap-2">
              <span className="break-all font-mono text-xs font-semibold">{f.path}</span>
              <button type="button" title="Comentar el fichero" aria-label="Comentar el fichero" onClick={() => { setPick({ file: f.path, start: -1, end: -1 }); setOpen(true); }} className="shrink-0 rounded p-0.5 text-slate-400 hover:bg-slate-100 hover:text-blue-600 dark:hover:text-accent-text dark:hover:bg-raised">
                <svg className="size-4" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round" aria-hidden="true">
                  <path d="M21 15a2 2 0 0 1-2 2H7l-4 4V5a2 2 0 0 1 2-2h14a2 2 0 0 1 2 2z" />
                  <path d="M12 7v6M9 10h6" />
                </svg>
              </button>
            </div>
            {pick?.file === f.path && pick.start < 0 && form(f, pick, `Comentario sobre ${f.path}…`)}
            <pre className="overflow-x-auto text-xs">
              {f.lines.map((l, i) => {
                const picked = pick?.file === f.path && i >= pick.start && i <= pick.end;
                return (
                  <div key={i}>
                    <div onMouseEnter={() => l.kind !== "hunk" && extend(f.path, i)} className={`flex ${picked ? "bg-yellow-100 dark:bg-yellow-900" : color[l.kind]}`}>
                      {l.kind === "hunk" ? <span className="w-20 shrink-0" /> : (
                        <button type="button" title="Comentar esta línea" onMouseDown={(e) => { e.preventDefault(); choose(f.path, i, e.shiftKey); }} onMouseEnter={() => extend(f.path, i)} className="w-20 shrink-0 select-none pr-2 text-right text-slate-400 hover:text-blue-600 dark:hover:text-accent-text">
                          <span className="inline-block w-8">{l.old ?? ""}</span> <span className="inline-block w-8">{l.new ?? ""}</span>
                        </button>
                      )}
                      <span>{l.text || " "}</span>
                    </div>
                    {open && !drag && pick?.file === f.path && pick.end === i && (
                      form(f, pick, `Comentario en líneas ${number(f.lines[pick.start])}–${number(l)}…`)
                    )}
                  </div>
                );
              })}
            </pre>
          </Card>
        ))}
      </div>
    </div>
  );
}
