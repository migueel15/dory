import { useEffect, useState } from "react";
import { useQueryClient } from "@tanstack/react-query";
import { noteThreadUpdate } from "./activity";
import type { Message, Queued, ThreadView, TurnModel } from "./client";

// useVisible tracks whether this tab is on screen. Live streams only stay open
// while it is: over plain HTTP a browser allows about six connections per host.
export function useVisible() {
  const [visible, setVisible] = useState(() => typeof document === "undefined" || document.visibilityState !== "hidden");
  useEffect(() => {
    const on = () => setVisible(document.visibilityState !== "hidden");
    document.addEventListener("visibilitychange", on);
    return () => document.removeEventListener("visibilitychange", on);
  }, []);
  return visible;
}

// Query keys refreshed when an entity changes, locally or replicated from another node.
const keys: Record<string, string[]> = {
  project: ["projects", "board"],
  repository: ["repositories", "board"],
  task: ["tasks", "board", "card"],
  task_dependency: ["board", "card", "dependencies"],
  plan: ["plans", "board", "card"],
  memory: ["memories"],
  thread: ["threads", "board", "card", "thread"],
  thread_state: ["threads", "board", "card", "thread"],
  notice: ["notices"],
  github: ["board", "card", "github", "github-detail"],
  browser: ["browser"],
};

// useGithubWatch keeps the node polling GitHub every few seconds while a view
// with branches or pull requests is on screen.
export function useGithubWatch() {
  const visible = useVisible();
  useEffect(() => {
    if (!visible) return;
    const beat = () => { fetch("/api/github/watch", { method: "POST", credentials: "include" }).catch(() => {}); };
    beat();
    const id = setInterval(beat, 20_000);
    return () => clearInterval(id);
  }, [visible]);
}

// The node version this page was loaded against. After an update the node
// restarts, the stream reconnects and a different version reloads the page,
// so the interface never runs against a newer core.
let loadedVersion: string | undefined;
function checkVersion() {
  fetch("/api/health", { credentials: "include", cache: "no-store" })
    .then((r) => (r.ok ? r.json() : undefined))
    .then((h?: { version?: string }) => {
      if (!h?.version) return;
      if (loadedVersion === undefined) loadedVersion = h.version;
      else if (h.version !== loadedVersion) window.location.reload();
    })
    .catch(() => {});
}

// useLiveUpdates subscribes to the node event stream and invalidates the
// affected queries, batching bursts so a sync triggers one refetch.
export function useLiveUpdates(enabled: boolean) {
  const qc = useQueryClient();
  const visible = useVisible();
  useEffect(() => {
    if (!enabled || !visible) return;
    const pending = new Set<string>();
    let timer: ReturnType<typeof setTimeout> | undefined;
    const source = new EventSource("/api/events", { withCredentials: true });
    let opened = false;
    // The epoch changes with each start of the node: another one means
    // everything may have changed while the stream was down.
    let epoch: string | undefined;
    source.addEventListener("hello", (e) => {
      const next = (JSON.parse((e as MessageEvent).data) as { epoch: string }).epoch;
      if (epoch !== undefined && epoch !== next) qc.invalidateQueries();
      epoch = next;
    });
    source.addEventListener("open", () => {
      checkVersion();
      if (opened) qc.invalidateQueries();
      opened = true;
    });
    source.addEventListener("change", (e) => {
      const { entity, id, data } = JSON.parse((e as MessageEvent).data) as { entity: string; id: string; data?: unknown };
      // An agent opened or closed a device: the thread page reacts.
      if (entity === "device") window.dispatchEvent(new CustomEvent("dory-device", { detail: { thread: id, ...(data as object) } }));
      if (entity === "browser" && (data as { action?: string } | undefined)?.action === "open") {
        window.dispatchEvent(new CustomEvent("dory-browser-open", { detail: id }));
      }
      for (const k of keys[entity] ?? []) pending.add(k);
      clearTimeout(timer);
      timer = setTimeout(() => {
        for (const k of pending) qc.invalidateQueries({ queryKey: [k] });
        pending.clear();
      }, 250);
    });
    // What each working thread is doing, shown on the board cards.
    source.addEventListener("thread", (e) => noteThreadUpdate(JSON.parse((e as MessageEvent).data)));
    return () => { clearTimeout(timer); source.close(); };
  }, [enabled, visible, qc]);
  const [seen, setSeen] = useState(visible);
  useEffect(() => { if (visible && !seen) qc.invalidateQueries(); setSeen(visible); }, [visible]); // eslint-disable-line react-hooks/exhaustive-deps
}

type ThreadEvent = { thread_id: string; seq?: number; type: string; item_id?: string; payload?: Record<string, unknown> };

// applyEvent folds one event of the thread into its cached view. Items are
// upserted by id, so replaying an event is harmless. Events that change more
// than the view carries (requests, settings, rewinds) refetch it.
function applyEvent(v: ThreadView, e: ThreadEvent): { view: ThreadView; refetch: boolean } {
  const next = { ...v };
  let refetch = false;
  const p = (e.payload ?? {}) as Record<string, any>; // eslint-disable-line @typescript-eslint/no-explicit-any
  switch (e.type) {
    case "item.started":
    case "item.completed":
    case "marker": {
      if (e.type === "marker" && p.kind === "rewind") { refetch = true; break; }
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
      else refetch = true;
      break;
    case "request.opened":
    case "request.resolved":
      refetch = true;
      break;
  }
  if (e.seq && e.seq > next.seq) next.seq = e.seq;
  return { view: next, refetch };
}

// applyDelta appends a fragment to the item being written, creating it when
// the client has not seen it yet.
function applyDelta(v: ThreadView, e: ThreadEvent): ThreadView {
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

// useThreadLive follows the stream of one thread: durable events (numbered)
// patch the cached view, fragments extend the text being written. EventSource
// reconnects with Last-Event-ID, so the server replays exactly what was missed.
export function useThreadLive(threadId: string | undefined) {
  const qc = useQueryClient();
  const visible = useVisible();
  useEffect(() => {
    if (!threadId || !visible) return;
    const key = ["thread", threadId];
    const source = new EventSource(`/api/threads/${encodeURIComponent(threadId)}/stream`, { withCredentials: true });
    let refetch: ReturnType<typeof setTimeout> | undefined;
    const later = () => {
      clearTimeout(refetch);
      refetch = setTimeout(() => qc.invalidateQueries({ queryKey: key }), 150);
    };
    // Without a past (first connection) the server says where the record is;
    // a view older than that is fetched again.
    source.addEventListener("snapshot", (e) => {
      const { seq } = JSON.parse((e as MessageEvent).data) as { seq: number };
      const v = qc.getQueryData<ThreadView>(key);
      if (!v || v.seq < seq) later();
    });
    source.addEventListener("thread", (e) => {
      const ev = JSON.parse((e as MessageEvent).data) as ThreadEvent;
      let again = false;
      qc.setQueryData<ThreadView>(key, (v) => {
        if (!v) { again = true; return v; }
        if (ev.seq && ev.seq <= v.seq) return v;
        const r = applyEvent(v, ev);
        again = r.refetch;
        return r.view;
      });
      if (again) later();
      // A fetch started before the turn ended may still answer running.
      if (ev.type === "turn.ended") later();
    });
    source.addEventListener("delta", (e) => {
      const ev = JSON.parse((e as MessageEvent).data) as ThreadEvent;
      qc.setQueryData<ThreadView>(key, (v) => (v ? applyDelta(v, ev) : v));
    });
    return () => { clearTimeout(refetch); source.close(); };
  }, [threadId, visible, qc]);
}
