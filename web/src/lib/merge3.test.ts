// @ts-nocheck -- runs with node:test; the project has no @types/node
import { test } from "node:test";
import assert from "node:assert/strict";
import { merge3 } from "./merge3.js";

const L = "Versión del agente";
const base = ["# Plan", "", "## Diseño", "", "Uno", "", "## Riesgos", "", "Ninguno"].join("\n");

test("changes in different sections combine", () => {
  const mine = base.replace("Uno", "Uno y medio");
  const theirs = base.replace("Ninguno", "Ninguno conocido");
  const r = merge3(base, mine, theirs, L);
  assert.equal(r.conflicts, 0);
  assert.equal(r.text, base.replace("Uno", "Uno y medio").replace("Ninguno", "Ninguno conocido"));
});

test("one side only takes that side", () => {
  assert.deepEqual(merge3(base, base, base + "\n- x", L), { text: base + "\n- x", conflicts: 0 });
  assert.deepEqual(merge3(base, base + "\n- y", base, L), { text: base + "\n- y", conflicts: 0 });
});

test("same change on both sides is not a conflict", () => {
  const both = base.replace("Uno", "Dos");
  assert.deepEqual(merge3(base, both, both, L), { text: both, conflicts: 0 });
});

test("the same line changed differently keeps both", () => {
  const r = merge3(base, base.replace("Uno", "Mío"), base.replace("Uno", "Suyo"), L);
  assert.equal(r.conflicts, 1);
  assert.ok(r.text.includes("Mío\n\n> **Versión del agente**\n> Suyo\n"));
  assert.ok(r.text.includes("Ninguno"));
});

test("insertions in different places combine", () => {
  const mine = base.replace("## Riesgos", "Nota mía\n\n## Riesgos");
  const theirs = base + "\n- añadido";
  const r = merge3(base, mine, theirs, L);
  assert.equal(r.conflicts, 0);
  assert.equal(r.text, mine + "\n- añadido");
});
