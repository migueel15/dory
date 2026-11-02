import { useMemo, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { api } from "../api/client";
import { Button, Card, ErrorText } from "../components/ui";

type Side = "LEFT" | "RIGHT";
type Line = { text: string; kind: "add" | "del" | "ctx" | "hunk"; old?: number; new?: number };
type File = { path: string; lines: Line[]; added: number; removed: number };
type Pick = { file: string; start: number; end: number };

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

const side = (l: Line): Side => (l.kind === "del" ? "LEFT" : "RIGHT");
const number = (l: Line) => (l.kind === "del" ? l.old! : l.new!);
const color = { add: "bg-green-100 dark:bg-green-950", del: "bg-red-100 dark:bg-red-950", hunk: "text-slate-500 dark:text-slate-400", ctx: "" };
const anchor = (path: string) => `diff-${path}`;

export function Diff({ repo, pr }: { repo: string; pr: number }) {
  const qc = useQueryClient();
  const diff = useQuery({ queryKey: ["github-diff", repo, pr], queryFn: () => api.githubDiff(repo, pr) });
  const files = useMemo(() => parseDiff(diff.data?.diff ?? ""), [diff.data]);
  const [filter, setFilter] = useState("");
  const [pick, setPick] = useState<Pick>();
  const [body, setBody] = useState("");
  const comment = useMutation({
    mutationFn: (a: Parameters<typeof api.githubAct>[3]) => api.githubAct(repo, "pulls", pr, a),
    onSuccess: () => {
      setPick(undefined); setBody("");
      qc.invalidateQueries({ queryKey: ["github-detail", repo, "pulls", pr] });
    },
  });
  const shown = files.filter((f) => f.path.toLowerCase().includes(filter.toLowerCase()));

  const choose = (file: string, i: number, shift: boolean) => {
    if (shift && pick?.file === file) setPick({ file, start: Math.min(pick.start, i), end: Math.max(pick.start, i) });
    else setPick({ file, start: i, end: i });
  };
  const publish = (f: File, p: Pick) => {
    const first = f.lines[p.start], last = f.lines[p.end];
    comment.mutate({ type: "comment_lines", body, path: f.path, line: number(last), side: side(last), start_line: number(first), start_side: side(first) });
  };
  const jump = (path: string) => document.getElementById(anchor(path))?.scrollIntoView({ behavior: "smooth", block: "start" });

  return (
    <div className="grid gap-3 md:grid-cols-[16rem_1fr]">
      <aside className="md:sticky md:top-2 md:max-h-[90vh] md:self-start md:overflow-auto">
        <Card>
          <input aria-label="Buscar fichero" value={filter} onChange={(e) => setFilter(e.target.value)} onKeyDown={(e) => e.key === "Enter" && shown[0] && jump(shown[0].path)} placeholder="Buscar fichero…" className="mb-2 w-full rounded border border-slate-300 bg-white px-2 py-1 text-sm dark:border-slate-700 dark:bg-slate-900" />
          <ul className="space-y-1 text-xs">
            {shown.map((f) => (
              <li key={f.path}>
                <button type="button" onClick={() => jump(f.path)} className="w-full break-all text-left hover:underline">
                  {f.path} <span className="text-green-700 dark:text-green-400">+{f.added}</span> <span className="text-red-700 dark:text-red-400">−{f.removed}</span>
                </button>
              </li>
            ))}
          </ul>
        </Card>
      </aside>
      <div className="min-w-0 space-y-3">
        <ErrorText error={diff.error ?? comment.error} />
        {pick === undefined && files.length > 0 && <p className="text-xs text-slate-500 dark:text-slate-400">Clic en un número de línea para comentarla; mayúsculas + clic para elegir un rango.</p>}
        {shown.map((f) => (
          <Card key={f.path}>
            <div id={anchor(f.path)} className="mb-1 scroll-mt-2 break-all font-mono text-xs font-semibold">{f.path}</div>
            <pre className="overflow-x-auto text-xs">
              {f.lines.map((l, i) => {
                const picked = pick?.file === f.path && i >= pick.start && i <= pick.end;
                return (
                  <div key={i}>
                    <div className={`flex ${picked ? "bg-yellow-100 dark:bg-yellow-900" : color[l.kind]}`}>
                      {l.kind === "hunk" ? <span className="w-20 shrink-0" /> : (
                        <button type="button" title="Comentar esta línea" onClick={(e) => choose(f.path, i, e.shiftKey)} className="w-20 shrink-0 select-none pr-2 text-right text-slate-400 hover:text-blue-600">
                          <span className="inline-block w-8">{l.old ?? ""}</span> <span className="inline-block w-8">{l.new ?? ""}</span>
                        </button>
                      )}
                      <span>{l.text || " "}</span>
                    </div>
                    {pick?.file === f.path && pick.end === i && (
                      <div className="my-2 whitespace-normal font-sans">
                        <textarea aria-label="Comentario de revisión" autoFocus value={body} onChange={(e) => setBody(e.target.value)} rows={3} placeholder={`Comentario en líneas ${number(f.lines[pick.start])}–${number(l)}…`} className="w-full rounded border border-slate-300 bg-white p-2 text-sm dark:border-slate-700 dark:bg-slate-900" />
                        <div className="flex gap-2">
                          <Button disabled={comment.isPending || !body.trim()} onClick={() => publish(f, pick)}>Publicar en GitHub</Button>
                          <Button variant="ghost" onClick={() => setPick(undefined)}>Cancelar</Button>
                        </div>
                      </div>
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
