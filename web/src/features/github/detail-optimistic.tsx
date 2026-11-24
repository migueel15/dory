import { useRef, useState } from "react";
import { useQueryClient } from "@tanstack/react-query";
import { api, type GithubAction, type GithubDetail } from "../../api/client";
import { qk } from "../../api/queries/keys";

// State changes show at once and are applied to the cached detail before GitHub answers.
export const logins = (d: GithubDetail, ids: string[] = []) => d.candidates.filter((u) => ids.includes(u.id));
// event appends what the viewer just wrote to the conversation, until GitHub returns the real one.
export const event = (d: GithubDetail, type: string, body = "", state = ""): GithubDetail =>
  ({ ...d, timeline: [...d.timeline, { id: `pending-${Date.now()}`, type, actor: d.viewer, at: new Date().toISOString(), body, state, detail: "" }] });
export const review = (d: GithubDetail, a: GithubAction, state: "APPROVED" | "CHANGES_REQUESTED"): GithubDetail => {
  if (!d.pull) return d;
  const base = a.body ? event(d, "PullRequestReview", a.body, state) : d;
  const approved = state === "APPROVED";
  return { ...base, pull: { ...d.pull,
    approved_by: approved ? [...new Set([...d.pull.approved_by, d.viewer])] : d.pull.approved_by.filter((l) => l !== d.viewer),
    changes_by: approved ? d.pull.changes_by.filter((l) => l !== d.viewer) : [...new Set([...d.pull.changes_by, d.viewer])],
    review_requests: d.pull.review_requests.filter((l) => l !== d.viewer) } };
};
export const editEvent = (d: GithubDetail, a: GithubAction): GithubDetail => ({ ...d, timeline: d.timeline.map((e) => (e.id === a.comment_id ? { ...e, body: a.body ?? e.body } : e)) });
export const editItem = (d: GithubDetail, a: GithubAction): GithubDetail => ({
  ...d, body: a.body ?? d.body,
  ...(d.pull && a.title ? { pull: { ...d.pull, title: a.title } } : {}),
  ...(d.issue && a.title ? { issue: { ...d.issue, title: a.title } } : {}),
});
export const thread = (d: GithubDetail, id: string | undefined, f: (t: GithubDetail["threads"][number]) => GithubDetail["threads"][number]): GithubDetail =>
  ({ ...d, threads: d.threads.map((t) => (t.id === id ? f(t) : t)) });

// Every action shows at once and is applied to the cached detail before GitHub answers.
export const optimistic: Record<string, (d: GithubDetail, a: GithubAction) => GithubDetail> = {
  assign: (d, a) => ({ ...d, assignees: [...d.assignees, ...logins(d, a.user_ids).filter((u) => !d.assignees.some((x) => x.id === u.id))] }),
  unassign: (d, a) => ({ ...d, assignees: d.assignees.filter((u) => !a.user_ids?.includes(u.id)) }),
  request_review: (d, a) => (d.pull ? { ...d, pull: { ...d.pull, review_requests: [...new Set([...d.pull.review_requests, ...logins(d, a.user_ids).map((u) => u.login)])] } } : d),
  approve: (d, a) => review(d, a, "APPROVED"),
  request_changes: (d, a) => review(d, a, "CHANGES_REQUESTED"),
  comment: (d, a) => event(d, "IssueComment", a.body),
  edit_comment: editEvent,
  edit_review: editEvent,
  edit_issue: editItem,
  edit_pull: editItem,
  set_milestone: (d, a) => (d.issue ? { ...d, issue: { ...d.issue, milestone: d.milestones.find((m) => m.id === a.milestone_id)?.title ?? "" } } : d),
  reply_thread: (d, a) => thread(d, a.thread_id, (t) => ({ ...t, comments: [...t.comments, { id: `pending-${Date.now()}`, author: d.viewer, at: new Date().toISOString(), body: a.body ?? "", url: "" }] })),
  resolve_thread: (d, a) => thread(d, a.thread_id, (t) => ({ ...t, resolved: true, resolved_by: d.viewer })),
  unresolve_thread: (d, a) => thread(d, a.thread_id, (t) => ({ ...t, resolved: false, resolved_by: "" })),
  ready: (d) => (d.pull ? { ...d, pull: { ...d.pull, draft: false } } : d),
  draft: (d) => (d.pull ? { ...d, pull: { ...d.pull, draft: true } } : d),
  close_pull: (d) => (d.pull ? { ...d, pull: { ...d.pull, state: "CLOSED" } } : d),
  reopen_pull: (d) => (d.pull ? { ...d, pull: { ...d.pull, state: "OPEN" } } : d),
  merge: (d) => (d.pull ? { ...d, pull: { ...d.pull, state: "MERGED" } } : d),
  close_issue: (d, a) => (d.issue ? { ...d, issue: { ...d.issue, state: "CLOSED", state_reason: a.method ?? "" } } : d),
  reopen_issue: (d) => (d.issue ? { ...d, issue: { ...d.issue, state: "OPEN", state_reason: "REOPENED" } } : d),
};

// useActionQueue sends GitHub actions one after another, in click order. When one fails,
// the detail is reloaded from GitHub (undoing the optimistic change) and the queued ones are dropped.
export function useActionQueue(key: unknown[], onDone: (a: GithubAction) => void) {
  const qc = useQueryClient();
  const tail = useRef<Promise<unknown>>(Promise.resolve());
  const epoch = useRef(0);
  const [pending, setPending] = useState(0);
  const inflight = useRef(0);
  const [error, setError] = useState<unknown>(null);
  const mutateAsync = (a: GithubAction) => {
    const [, full, kind, n] = key as [string, string, string, number];
    const mine = epoch.current;
    setError(null);
    const patch = optimistic[a.type];
    if (patch) { qc.cancelQueries({ queryKey: key }); qc.setQueryData<GithubDetail>(key, (d) => (d ? patch(d, a) : d)); }
    inflight.current++;
    setPending((p) => p + 1);
    const run = tail.current.catch(() => {}).then(async () => {
      if (epoch.current !== mine) throw new Error("Cancelada: falló una acción anterior.");
      try {
        await api.githubAct(full, kind, n, a);
        onDone(a);
      } catch (e) {
        epoch.current++;
        setError(e);
        throw e;
      }
    }).finally(() => {
      setPending((p) => p - 1);
      // Reload only once the queue is empty, so a later optimistic change is not overwritten.
      if (--inflight.current === 0) {
        qc.invalidateQueries({ queryKey: key });
        qc.invalidateQueries({ queryKey: qk.github() });
      } else qc.cancelQueries({ queryKey: key });
    });
    tail.current = run;
    return run;
  };
  return { mutateAsync, mutate: (a: GithubAction) => { mutateAsync(a).catch(() => {}); }, isPending: pending > 0, error };
}

