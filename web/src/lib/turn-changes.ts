// The changes of one agent turn, built from the files its edit calls reported.

export type TurnFile = { path: string; change?: string; added: number; removed: number; diff?: string };
type Edit = { path: string; change?: string; added: number; removed: number; diff?: string };

// mergeTurnFiles joins a turn's edits by path: it adds their counts, keeps the
// last change and concatenates their hunks in order.
export function mergeTurnFiles(edits: Edit[]): TurnFile[] {
  const by = new Map<string, TurnFile>();
  for (const e of edits) {
    const f = by.get(e.path);
    if (!f) { by.set(e.path, { path: e.path, change: e.change, added: e.added, removed: e.removed, diff: e.diff || undefined }); continue; }
    f.added += e.added;
    f.removed += e.removed;
    // A file created and then edited in the same turn is still new.
    if (e.change && !(f.change === "add" && e.change === "update")) f.change = e.change;
    if (e.diff) f.diff = f.diff ? `${f.diff}\n${e.diff}` : e.diff;
  }
  return [...by.values()];
}

// turnDiff writes the turn's files as one unified diff the diff viewer reads.
// Harnesses send partial hunks, some without numbered headers; those get one.
export function turnDiff(files: TurnFile[]): string {
  const out: string[] = [];
  for (const f of files) {
    out.push(`diff --git a/${f.path} b/${f.path}`);
    let open = false;
    for (const l of (f.diff ?? "").split("\n")) {
      if (/^(diff --git |index |--- |\+\+\+ )/.test(l)) continue;
      if (l.startsWith("@@")) { out.push(/^@@ -\d+/.test(l) ? l : "@@ -1 +1 @@"); open = true; continue; }
      if (!l) continue;
      if (!open) { out.push("@@ -1 +1 @@"); open = true; }
      out.push(/^[+\- ]/.test(l) ? l : ` ${l}`);
    }
  }
  return out.join("\n");
}
