import { side, number } from "./diff-parts";

export type Side = "LEFT" | "RIGHT";
export type Line = { text: string; kind: "add" | "del" | "ctx" | "hunk"; old?: number; new?: number };
export type File = { path: string; lines: Line[]; added: number; removed: number };
export type Pick = { file: string; start: number; end: number }; // start -1: whole file

// patchHash fingerprints a file patch so a review mark expires when the file changes.
export async function patchHash(f: File): Promise<string> {
  const data = new TextEncoder().encode(f.lines.map((l) => l.text).join("\n"));
  return Array.from(new Uint8Array(await crypto.subtle.digest("SHA-1", data)), (b) => b.toString(16).padStart(2, "0")).join("");
}

// parseDiff splits a unified diff into files with old and new line numbers.
export function parseDiff(diff: string): File[] {
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

export type Row = { hunk?: number; left?: number; right?: number };

// splitRows pairs removed and added lines side by side, like GitHub's split view.
export function splitRows(lines: Line[]): Row[] {
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

export type Dir<F = File> = { name: string; dirs: Dir<F>[]; files: F[] };

// tree groups files by folder and joins folders that hold a single folder.
export function tree<F extends { path: string } = File>(files: F[]): Dir<F> {
  const root: Dir<F> = { name: "", dirs: [], files: [] };
  for (const f of files) {
    let d = root;
    for (const part of f.path.split("/").slice(0, -1)) {
      let next = d.dirs.find((x) => x.name === part);
      if (!next) d.dirs.push((next = { name: part, dirs: [], files: [] }));
      d = next;
    }
    d.files.push(f);
  }
  const squash = (d: Dir<F>): Dir<F> => {
    d.dirs = d.dirs.map(squash);
    if (d.name && d.dirs.length === 1 && !d.files.length) return { ...d.dirs[0], name: `${d.name}/${d.dirs[0].name}` };
    return d;
  };
  return squash(root);
}

export type Review = { reviewed: Set<string>; toggle?: (path: string) => void };

