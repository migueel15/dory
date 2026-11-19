// Three-way merge of texts by lines, as git merges two branches: changes of
// each side to a common base are combined; when both sides change the same
// lines differently, both versions are kept, the other side's one quoted below
// under a label, so nothing is lost.

type Hunk = { start: number; end: number; lines: string[] };

// diff gives the ranges of base replaced to get other, from a longest common
// subsequence of lines.
function diff(base: string[], other: string[]): Hunk[] {
  const n = base.length, m = other.length;
  const lcs = Array.from({ length: n + 1 }, () => new Uint32Array(m + 1));
  for (let i = n - 1; i >= 0; i--) {
    for (let j = m - 1; j >= 0; j--) lcs[i][j] = base[i] === other[j] ? lcs[i + 1][j + 1] + 1 : Math.max(lcs[i + 1][j], lcs[i][j + 1]);
  }
  const hunks: Hunk[] = [];
  let i = 0, j = 0, open: { start: number; from: number } | undefined;
  const close = () => { if (open) { hunks.push({ start: open.start, end: i, lines: other.slice(open.from, j) }); open = undefined; } };
  while (i < n || j < m) {
    if (i < n && j < m && base[i] === other[j]) { close(); i++; j++; continue; }
    open ??= { start: i, from: j };
    if (j < m && (i === n || lcs[i][j + 1] >= lcs[i + 1][j])) j++;
    else i++;
  }
  close();
  return hunks;
}

// apply replaces, in base[start, end), the hunks of one side that fall in it.
function apply(base: string[], start: number, end: number, hunks: Hunk[]) {
  const out: string[] = [];
  let at = start;
  for (const h of hunks) {
    out.push(...base.slice(at, h.start), ...h.lines);
    at = h.end;
  }
  out.push(...base.slice(at, end));
  return out;
}

export type Merge = { text: string; conflicts: number };

// merge3 combines mine and theirs, both changed from base. label heads the
// quoted copy of theirs where the two disagree.
export function merge3(base: string, mine: string, theirs: string, label: string): Merge {
  if (mine === base || mine === theirs) return { text: theirs, conflicts: 0 };
  if (theirs === base) return { text: mine, conflicts: 0 };
  const b = base.split("\n");
  const all = [
    ...diff(b, mine.split("\n")).map((h) => ({ ...h, side: 0 })),
    ...diff(b, theirs.split("\n")).map((h) => ({ ...h, side: 1 })),
  ].sort((x, y) => x.start - y.start || x.end - y.end);
  const out: string[] = [];
  let at = 0, conflicts = 0;
  for (let k = 0; k < all.length;) {
    // A group gathers the hunks that overlap, or insert at the same place.
    const group = [all[k]];
    let start = all[k].start, end = all[k].end;
    for (k++; k < all.length; k++) {
      const h = all[k];
      // Overlapping ranges, or an insertion where the group starts or inserts too.
      if (h.start < end || (h.start === end && start === end) || (h.start === h.end && h.start === start)) {
        group.push(h);
        end = Math.max(end, h.end);
        start = Math.min(start, h.start);
      } else break;
    }
    out.push(...b.slice(at, start));
    const ours = group.filter((h) => h.side === 0), others = group.filter((h) => h.side === 1);
    const a = apply(b, start, end, ours), t = apply(b, start, end, others);
    if (!others.length) out.push(...a);
    else if (!ours.length || a.join("\n") === t.join("\n")) out.push(...t);
    else {
      conflicts++;
      out.push(...a, "", `> **${label}**`, ...t.map((l) => (l ? `> ${l}` : ">")), "");
    }
    at = end;
  }
  out.push(...b.slice(at));
  return { text: out.join("\n"), conflicts };
}
