// @ts-nocheck -- runs with node:test; the project has no @types/node
import assert from "node:assert/strict";
import { test } from "node:test";
import { needsRefetch, reduceDelta, reduceEvent, reduceThread, type ThreadEvent } from "./thread-reducer.js";
import type { ThreadView } from "./types.gen.js";

const view = (over: Partial<ThreadView> = {}) =>
  ({ id: "t1", seq: 5, status: "idle", running: false, messages: [], queue: [], ...over }) as unknown as ThreadView;
const ev = (type: string, payload: Record<string, unknown>, seq?: number): ThreadEvent => ({ thread_id: "t1", type, seq, payload });

test("an item is added and then replaced by id", () => {
  let v = reduceEvent(view(), ev("item.started", { id: "m1", text: "a" }, 6));
  assert.equal(v.messages.length, 1);
  assert.equal(v.seq, 6);
  v = reduceEvent(v, ev("item.completed", { id: "m1", text: "ab" }, 7));
  assert.equal(v.messages.length, 1);
  assert.equal(v.messages[0].text, "ab");
});

test("an event at or before the seq is ignored", () => {
  const v = view();
  assert.equal(reduceEvent(v, ev("item.started", { id: "m1" }, 5)), v);
});

test("state.changed sets status and running", () => {
  const v = reduceEvent(view(), ev("state.changed", { status: "working" }, 6));
  assert.equal(v.status, "working");
  assert.equal(v.running, true);
});

test("queue.changed replaces the queue", () => {
  const v = reduceEvent(view(), ev("queue.changed", { queue: [{ run_id: "r" }] }, 6));
  assert.equal(v.queue.length, 1);
  assert.equal(reduceEvent(v, ev("queue.changed", {}, 7)).queue.length, 0);
});

test("the reducer does not mutate its input", () => {
  const v = view();
  reduceEvent(v, ev("item.started", { id: "m1" }, 6));
  assert.equal(v.messages.length, 0);
  assert.equal(v.seq, 5);
});

test("deltas create the item and extend it, or replace it", () => {
  let v = reduceDelta(view(), ev("delta", { id: "m1", role: "assistant", text: "ho" }));
  assert.equal(v.messages[0].text, "ho");
  assert.equal(v.messages[0].streaming, true);
  v = reduceThread(v, ev("delta", { id: "m1", text: "la" }), "delta");
  assert.equal(v.messages[0].text, "hola");
  v = reduceDelta(v, ev("delta", { id: "m1", text: "x", replace: true }));
  assert.equal(v.messages[0].text, "x");
  assert.equal(v.seq, 5);
});

test("which events refetch the view", () => {
  assert.equal(needsRefetch(ev("marker", { kind: "rewind" })), true);
  assert.equal(needsRefetch(ev("marker", { kind: "clear", id: "m" })), false);
  assert.equal(needsRefetch(ev("request.opened", {})), true);
  assert.equal(needsRefetch(ev("settings.reported", {})), true);
  assert.equal(needsRefetch(ev("settings.reported", { last_turn_model: {} })), false);
  assert.equal(needsRefetch(ev("item.started", { id: "m" })), false);
});
