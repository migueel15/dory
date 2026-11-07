// @ts-nocheck -- runs with node:test; the project has no @types/node
import { test } from "node:test";
import assert from "node:assert/strict";
import { buildStacks, groupByStack, pullKey } from "./stack.js";

const pr = (number: number, head_ref: string, base_ref: string, repo = "o/r", state = "OPEN") => ({ repo, number, state, head_ref, base_ref });
const order = (pulls: ReturnType<typeof pr>[], n: number) => buildStacks(pulls).get(pullKey(pulls.find((p) => p.number === n)!))?.nodes.map((x) => `${x.pull.number}:${x.depth}`);

test("sin stack", () => {
  assert.equal(buildStacks([pr(1, "a", "main"), pr(2, "b", "main")]).size, 0);
});
test("cadena lineal de 3", () => {
  const l = [pr(3, "c", "b"), pr(1, "a", "main"), pr(2, "b", "a")];
  assert.deepEqual(order(l, 2), ["1:0", "2:1", "3:2"]);
  const m = buildStacks(l);
  assert.equal(m.get("o/r#3"), m.get("o/r#1"));
});
test("bifurcación como árbol", () => {
  assert.deepEqual(order([pr(1, "a", "main"), pr(2, "b", "a"), pr(3, "c", "a"), pr(4, "d", "b")], 1), ["1:0", "2:1", "4:2", "3:1"]);
});
test("ciclo y ref huérfana", () => {
  assert.equal(buildStacks([pr(1, "a", "b"), pr(2, "b", "a")]).size, 0);
  assert.equal(buildStacks([pr(1, "a", "gone")]).size, 0);
});
test("repos distintos y PRs cerradas no se mezclan", () => {
  assert.equal(buildStacks([pr(1, "a", "main"), pr(2, "b", "a", "o/otro")]).size, 0);
  assert.equal(buildStacks([pr(1, "a", "main", "o/r", "MERGED"), pr(2, "b", "a")]).size, 0);
});
test("agrupa donde aparece el miembro más reciente", () => {
  const l = [pr(9, "x", "main"), pr(3, "c", "b"), pr(7, "y", "main"), pr(1, "a", "main"), pr(2, "b", "a")];
  const g = groupByStack(l, buildStacks(l));
  assert.deepEqual(g.map((x) => x.pull.number), [9, 1, 2, 3, 7]);
  assert.equal(g[3].label, "stack · 3/3");
  assert.equal(g[0].label, undefined);
});
