// Pure reducer of the thread view: folds the events of the thread stream into
// the cached view. No React, no cache: the same input gives the same view.
import type { Message, Queued, ThreadView, TurnModel } from "./types.gen.js";

export type ThreadEvent = { thread_id: string; seq?: number; type: string; item_id?: string; payload?: Record<string, unknown> };

// needsRefetch says whether an event changes more than the view carries
// (requests, settings, rewinds): the view is fetched again after folding it.
export function needsRefetch(e: ThreadEvent): boolean {
  const p = e.payload ?? {};
  switch (e.type) {
    case "marker": return p.kind === "rewind";
    case "settings.reported": return !p.last_turn_model;
    case "request.opened":
    case "request.resolved": return true;
    default: return false;
  }
}

// reduceEvent folds one durable event (numbered) into the view. Items are
// upserted by id, so replaying an event is harmless; an event at or before
// the view's seq is ignored.
export function reduceEvent(v: ThreadView, e: ThreadEvent): ThreadView {
  if (e.seq && e.seq <= v.seq) return v;
  const next = { ...v };
  const p = (e.payload ?? {}) as Record<string, any>; // eslint-disable-line @typescript-eslint/no-explicit-any
  switch (e.type) {
    case "item.started":
    case "item.completed":
    case "marker": {
      if (e.type === "marker" && p.kind === "rewind") break;
      if (!p.id) break;
      const msg = p as unknown as Message;
      const i = v.messages.findIndex((m) => m.id === msg.id);
      next.messages = i < 0 ? [...v.messages, msg] : v.messages.map((m, j) => (j === i ? msg : m));
      break;
    }
    case "state.changed":
      next.status = p.status as ThreadView["status"];
      next.running = p.status === "working";
      break;
    case "queue.changed":
      next.queue = (p.queue as Queued[] | undefined) ?? [];
      break;
    case "settings.reported":
      if (p.last_turn_model) next.last_turn_model = p.last_turn_model as TurnModel;
      break;
  }
  if (e.seq && e.seq > next.seq) next.seq = e.seq;
  return next;
}

// reduceDelta appends a fragment to the item being written, creating it when
// the client has not seen it yet. Fragments carry no seq: they never move it.
export function reduceDelta(v: ThreadView, e: ThreadEvent): ThreadView {
  const p = (e.payload ?? {}) as { id: string; role: string; run_id?: string; created_at?: string; text: string; replace?: boolean };
  const i = v.messages.findIndex((m) => m.id === p.id);
  if (i < 0) {
    const msg = { id: p.id, thread_id: v.id, run_id: p.run_id, role: p.role, text: p.text, streaming: true, created_at: p.created_at ?? "" } as Message;
    return { ...v, messages: [...v.messages, msg] };
  }
  const cur = v.messages[i];
  const text = p.replace ? p.text : cur.text + p.text;
  return { ...v, messages: v.messages.map((m, j) => (j === i ? { ...cur, text, streaming: true } : m)) };
}

// reduceThread is the entry point: durable events and deltas, by event type.
export function reduceThread(v: ThreadView, e: ThreadEvent, kind: "thread" | "delta" = "thread"): ThreadView {
  return kind === "delta" ? reduceDelta(v, e) : reduceEvent(v, e);
}
