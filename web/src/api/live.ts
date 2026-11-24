import { useEffect, useState } from "react";
import { useQueryClient } from "@tanstack/react-query";
import { noteThreadUpdate } from "./activity";
import type { ThreadView } from "./types.gen";
import { qk } from "./queries/keys";
import { deltaGap, needsRefetch, reduceDelta, reduceEvent, type ThreadEvent } from "./thread-reducer";
import { events, type DeviceEvent, type TerminalEvent } from "../lib/store";

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

// queriesFor maps an entity change to the exact query keys it affects.
// Plans and dependencies carry their own ids, not the task's, so they refresh
// the family. A key is refreshed once per burst (see useLiveUpdates).
export function queriesFor(entity: string, id: string): unknown[][] {
  switch (entity) {
    case "project": return [qk.projects(), qk.board()];
    case "repository": return [qk.repositories(), qk.board()];
    case "task": return [qk.card(id), qk.board()];
    case "task_dependency": return [qk.board(), qk.card(), qk.dependencies()];
    case "plan": return [qk.plans(), qk.board(), qk.card()];
    case "memory": return [qk.memories()];
    case "thread":
    case "thread_state": return [qk.thread(id), qk.threads(), qk.board(), qk.card()];
    case "terminal": return [qk.threadTerminals(id)];
    case "notice": return [qk.notices()];
    case "github": return [qk.board(), qk.card(), qk.github(), qk.githubDetail()];
    default: return [];
  }
}

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
    const pending = new Map<string, unknown[]>();
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
      if (entity === "device") events.deviceChanged.emit({ thread: id, ...(data as object) } as DeviceEvent);
      if (entity === "terminal" && data) events.terminalChanged.emit({ thread: id, ...(data as object) } as TerminalEvent);
      if (entity === "browser" && (data as { action?: string } | undefined)?.action === "open") {
        events.browserOpened.emit(id);
      }
      for (const k of queriesFor(entity, id)) pending.set(JSON.stringify(k), k);
      clearTimeout(timer);
      timer = setTimeout(() => {
        for (const k of pending.values()) qc.invalidateQueries({ queryKey: k });
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

// useThreadLive follows the stream of one thread: durable events (numbered)
// patch the cached view, fragments extend the text being written. EventSource
// reconnects with Last-Event-ID, so the server replays exactly what was missed.
export function useThreadLive(threadId: string | undefined) {
  const qc = useQueryClient();
  const visible = useVisible();
  useEffect(() => {
    if (!threadId || !visible) return;
    const key = qk.thread(threadId);
    const url = `/api/threads/${encodeURIComponent(threadId)}/stream`;
    let source: EventSource;
    let refetch: ReturnType<typeof setTimeout> | undefined;
    const later = () => {
      clearTimeout(refetch);
      refetch = setTimeout(() => qc.invalidateQueries({ queryKey: key }), 150);
    };
    // A fragment after a gap: reconnecting from the view's seq brings what
    // was missed and the texts being written, whole.
    let reconnecting = false;
    const reconnect = () => {
      if (reconnecting) return;
      reconnecting = true;
      source.close();
      const v = qc.getQueryData<ThreadView>(key);
      connect(v ? `${url}?after=${v.seq}` : url);
    };
    const connect = (at: string) => {
      source = new EventSource(at, { withCredentials: true });
      source.addEventListener("open", () => { reconnecting = false; });
      // Without a past (first connection) the server says where the record
      // is; a view older than that is fetched again.
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
          again = needsRefetch(ev);
          return reduceEvent(v, ev);
        });
        if (again) later();
        // A fetch started before the turn ended may still answer running.
        if (ev.type === "turn.ended") later();
      });
      source.addEventListener("delta", (e) => {
        const ev = JSON.parse((e as MessageEvent).data) as ThreadEvent;
        let gap = false;
        qc.setQueryData<ThreadView>(key, (v) => {
          if (!v) return v;
          gap = deltaGap(v, ev);
          return reduceDelta(v, ev);
        });
        if (gap) reconnect();
      });
    };
    connect(url);
    return () => { clearTimeout(refetch); source.close(); };
  }, [threadId, visible, qc]);
}
