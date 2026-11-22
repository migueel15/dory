// @ts-nocheck -- runs with node:test; the project has no @types/node
import { test } from "node:test";
import assert from "node:assert/strict";
import { mergeTurnFiles, turnDiff } from "./turn-changes.js";

test("mergeTurnFiles joins edits by path", () => {
  const files = mergeTurnFiles([
    { path: "a.ts", change: "add", added: 3, removed: 0, diff: "+a" },
    { path: "b.ts", change: "update", added: 1, removed: 1, diff: "-x\n+y" },
    { path: "a.ts", change: "update", added: 2, removed: 1, diff: "+b" },
    { path: "c.ts", change: "delete", added: 0, removed: 4 },
  ]);
  assert.deepEqual(files, [
    { path: "a.ts", change: "add", added: 5, removed: 1, diff: "+a\n+b" },
    { path: "b.ts", change: "update", added: 1, removed: 1, diff: "-x\n+y" },
    { path: "c.ts", change: "delete", added: 0, removed: 4, diff: undefined },
  ]);
});

test("turnDiff adds headers the viewer needs", () => {
  assert.equal(turnDiff([{ path: "a.ts", added: 1, removed: 1, diff: "--- a/a.ts\n+++ b/a.ts\n-x\n+y" }]),
    "diff --git a/a.ts b/a.ts\n@@ -1 +1 @@\n-x\n+y");
  assert.equal(turnDiff([{ path: "a.ts", added: 1, removed: 0, diff: "@@ -3,2 +3,3 @@\n+y" }]),
    "diff --git a/a.ts b/a.ts\n@@ -3,2 +3,3 @@\n+y");
});
