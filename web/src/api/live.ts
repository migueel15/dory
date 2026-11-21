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
  notice: ["notices"],
  github: ["board", "card", "github", "github-detail"],
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

type Update = { thread_id: string; message?: Message; status?: string; error?: string; queue?: Queued[]; reload?: boolean; last_turn_model?: TurnModel };

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
    source.addEventListener("open", () => {
      checkVersion();
      if (opened) qc.invalidateQueries();
      opened = true;
    });
    source.addEventListener("change", (e) => {
      const { entity } = JSON.parse((e as MessageEvent).data) as { entity: string };
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

// useThreadLive patches the cached view of one thread with each streamed
// message, so tokens appear as the agent writes them.
export function useThreadLive(threadId: string | undefined) {
  const qc = useQueryClient();
  const visible = useVisible();
  useEffect(() => {
    if (!threadId || !visible) return;
    const key = ["thread", threadId];
    const source = new EventSource(`/api/events?thread=${encodeURIComponent(threadId)}`, { withCredentials: true });
    source.addEventListener("thread", (e) => {
      const u = JSON.parse((e as MessageEvent).data) as Update;
      if (u.thread_id !== threadId) return;
      // The history was rebuilt from the harness file: fetch it whole.
      if (u.reload) { qc.invalidateQueries({ queryKey: key }); return; }
      qc.setQueryData<ThreadView>(key, (v) => {
        if (!v) return v;
        const next = { ...v };
        if (u.message) {
          const i = v.messages.findIndex((m) => m.id === u.message!.id);
          next.messages = i < 0 ? [...v.messages, u.message] : v.messages.map((m, j) => (j === i ? u.message! : m));
        }
        if (u.status) {
          next.status = u.status as ThreadView["status"];
          next.running = u.status === "working";
        }
        if (u.queue) next.queue = u.queue;
        if (u.last_turn_model) next.last_turn_model = u.last_turn_model;
        return next;
      });
    });
    // Whatever changed while the stream was down is reloaded on every (re)open.
    source.addEventListener("open", () => { qc.invalidateQueries({ queryKey: key }); });
    return () => source.close();
  }, [threadId, visible, qc]);
}
