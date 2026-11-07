// Stacks of pull requests: a pull request whose base is the head branch of
// another open pull request of the same repository sits on top of it.
export type StackPull = { repo: string; number: number; state: string; head_ref: string; base_ref: string };
export type StackNode<T extends StackPull> = { pull: T; depth: number };
export type Stack<T extends StackPull> = { nodes: StackNode<T>[] };

export const pullKey = (p: { repo: string; number: number }) => `${p.repo}#${p.number}`;

// buildStacks finds the stacks among the open pull requests and returns them
// by the key of each member. Nodes go from the base to the top, depth-first, so
// a fork in the chain reads as an indented tree. Pull requests outside any
// chain, and refs that form a cycle, get no stack.
export function buildStacks<T extends StackPull>(pulls: T[]): Map<string, Stack<T>> {
  const open = pulls.filter((p) => p.state === "OPEN");
  const byHead = new Map<string, T[]>();
  for (const p of open) {
    const k = `${p.repo}\0${p.head_ref}`;
    byHead.set(k, [...(byHead.get(k) ?? []), p]);
  }
  const parentOf = (p: T) => (byHead.get(`${p.repo}\0${p.base_ref}`) ?? []).find((q) => q.number !== p.number);
  const children = new Map<string, T[]>();
  const roots: T[] = [];
  for (const p of open) {
    const parent = parentOf(p);
    if (parent) children.set(pullKey(parent), [...(children.get(pullKey(parent)) ?? []), p]);
    else roots.push(p);
  }
  const out = new Map<string, Stack<T>>();
  for (const root of roots) {
    const nodes: StackNode<T>[] = [];
    const walk = (p: T, depth: number) => {
      nodes.push({ pull: p, depth });
      [...(children.get(pullKey(p)) ?? [])].sort((a, b) => a.number - b.number).forEach((c) => walk(c, depth + 1));
    };
    walk(root, 0);
    if (nodes.length < 2) continue;
    const stack = { nodes };
    nodes.forEach((n) => out.set(pullKey(n.pull), stack));
  }
  return out;
}

export type Grouped<T extends StackPull> = { pull: T; depth: number; label?: string };

// groupByStack keeps the order of the list but gathers each stack where its
// first member appears, which is the most recent one when the list is sorted
// by activity. Members the list does not contain are left out.
export function groupByStack<T extends StackPull>(list: T[], stacks: Map<string, Stack<T>>): Grouped<T>[] {
  const shown = new Set(list.map(pullKey));
  const done = new Set<Stack<T>>();
  const out: Grouped<T>[] = [];
  for (const p of list) {
    const s = stacks.get(pullKey(p));
    if (!s) { out.push({ pull: p, depth: 0 }); continue; }
    if (done.has(s)) continue;
    done.add(s);
    s.nodes.forEach((n, i) => {
      if (shown.has(pullKey(n.pull))) out.push({ pull: list.find((x) => pullKey(x) === pullKey(n.pull))!, depth: n.depth, label: `stack · ${i + 1}/${s.nodes.length}` });
    });
  }
  return out;
}
