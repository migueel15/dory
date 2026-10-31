import { useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Link, useParams } from "@tanstack/react-router";
import { api, type GithubDetail, type GithubPull } from "../api/client";
import { Badge, Button, Card, ErrorText, Md } from "../components/ui";
import { PullStatus } from "./github";

const labels: Record<string, string> = {
  IssueComment: "comentó", PullRequestReview: "revisó", PullRequestCommit: "hizo un commit", ClosedEvent: "cerró", ReopenedEvent: "reabrió", MergedEvent: "mergeó", LabeledEvent: "añadió la etiqueta", AssignedEvent: "asignó", ReadyForReviewEvent: "marcó como lista", ConvertToDraftEvent: "pasó a draft", ReviewRequestedEvent: "pidió revisión", CrossReferencedEvent: "referenció desde",
};
const reviewStates: Record<string, string> = { APPROVED: "aprobó", CHANGES_REQUESTED: "pidió cambios", COMMENTED: "comentó en la revisión", DISMISSED: "revisión descartada" };

function lineColor(l: string) {
  if (l.startsWith("+") && !l.startsWith("+++")) return "bg-green-100 dark:bg-green-950";
  if (l.startsWith("-") && !l.startsWith("---")) return "bg-red-100 dark:bg-red-950";
  if (l.startsWith("@@") || l.startsWith("diff ")) return "text-slate-500 dark:text-slate-400";
  return "";
}

function Diff({ repo, number }: { repo: string; number: number }) {
  const diff = useQuery({ queryKey: ["github-diff", repo, number], queryFn: () => api.githubDiff(repo, number) });
  return (
    <Card>
      <ErrorText error={diff.error} />
      <pre className="max-h-[70vh] overflow-auto text-xs">{diff.data?.diff.split("\n").map((l, i) => <div key={i} className={lineColor(l)}>{l || " "}</div>)}</pre>
    </Card>
  );
}

function PullActions({ pull, act, busy }: { pull: GithubPull; act: (type: string, extra?: { method?: string }) => void; busy: boolean }) {
  const [method, setMethod] = useState("MERGE");
  if (pull.state !== "OPEN") return pull.state === "CLOSED" ? <Button disabled={busy} onClick={() => act("reopen_pull")}>Reabrir</Button> : null;
  return (
    <>
      {pull.draft ? <Button disabled={busy} onClick={() => act("ready")}>Marcar como lista</Button> : <Button variant="ghost" disabled={busy} onClick={() => act("draft")}>Pasar a draft</Button>}
      <Button disabled={busy} onClick={() => act("approve")}>Aprobar</Button>
      <select aria-label="Método de merge" value={method} onChange={(e) => setMethod(e.target.value)} className="rounded border border-slate-300 bg-white px-2 text-sm dark:border-slate-700 dark:bg-slate-900">
        <option value="MERGE">Merge commit</option><option value="SQUASH">Squash</option><option value="REBASE">Rebase</option>
      </select>
      <Button disabled={busy} onClick={() => window.confirm(`¿Mergear ${pull.repo}#${pull.number} en ${pull.base_ref}?`) && act("merge", { method })}>Mergear</Button>
      <Button variant="danger" disabled={busy} onClick={() => window.confirm("¿Cerrar la PR sin mergear?") && act("close_pull")}>Cerrar</Button>
    </>
  );
}

function Header({ d }: { d: GithubDetail }) {
  const e = d.pull ?? d.issue!;
  return (
    <Card>
      <div className="flex items-start justify-between gap-2">
        <h1 className="min-w-0 break-words text-lg font-bold">{e.title}</h1>
        <a href={e.url} target="_blank" rel="noreferrer" className="shrink-0 text-sm underline">Abrir en GitHub</a>
      </div>
      <div className="text-xs text-slate-500 dark:text-slate-400">{e.repo}#{e.number} · {e.author}{d.pull && ` · ${d.pull.head_ref} → ${d.pull.base_ref}`}</div>
      <div className="mt-1 flex flex-wrap gap-1"><Badge>{e.state}</Badge>{e.labels.map((l) => <Badge key={l}>{l}</Badge>)}</div>
      {d.pull && <div className="mt-2"><PullStatus pull={d.pull} /></div>}
      {d.pull && d.pull.approved_by.length > 0 && <div className="text-xs text-slate-500 dark:text-slate-400">Aprobada por {d.pull.approved_by.join(", ")}</div>}
      {(d.pull?.issues ?? d.issue?.pulls ?? []).map((r) => {
        const [owner, repo] = e.repo.split("/");
        return <Link key={r.number} className="mr-2 text-xs underline" to="/github/$owner/$repo/$kind/$number" params={{ owner, repo, kind: d.pull ? "issues" : "pulls", number: String(r.number) }}>#{r.number} {r.title}</Link>;
      })}
    </Card>
  );
}

export function GithubView() {
  const { owner, repo, kind, number } = useParams({ strict: false }) as Record<string, string>;
  const full = `${owner}/${repo}`;
  const n = Number(number);
  const qc = useQueryClient();
  const [text, setText] = useState("");
  const [showDiff, setShowDiff] = useState(false);
  const key = ["github-detail", full, kind, n];
  const detail = useQuery({ queryKey: key, queryFn: () => api.githubDetail(full, kind, n), refetchInterval: 60_000 });
  const action = useMutation({
    mutationFn: (a: { type: string; body?: string; method?: string }) => api.githubAct(full, kind, n, a),
    onSuccess: (_, a) => {
      if (a.body) setText("");
      qc.invalidateQueries({ queryKey: key });
      qc.invalidateQueries({ queryKey: ["github"] });
    },
  });
  const d = detail.data;
  const act = (type: string, extra = {}) => action.mutate({ type, ...extra });
  const open = (d?.pull ?? d?.issue)?.state === "OPEN";
  return (
    <div className="space-y-3">
      <Link to="/github" className="text-sm underline">← GitHub</Link>
      <ErrorText error={detail.error ?? action.error} />
      {d && <>
        <Header d={d} />
        <div className="flex flex-wrap gap-2">
          {d.pull && <PullActions pull={d.pull} act={act} busy={action.isPending} />}
          {d.issue && <Button variant={open ? "danger" : "default"} disabled={action.isPending} onClick={() => act(open ? "close_issue" : "reopen_issue")}>{open ? "Cerrar" : "Reabrir"}</Button>}
          {d.pull && <Button variant="ghost" onClick={() => setShowDiff(!showDiff)}>{showDiff ? "Ocultar diff" : "Ver diff"}</Button>}
        </div>
        {showDiff && <Diff repo={full} number={n} />}
        {d.body && <Card><Md>{d.body}</Md></Card>}
        {d.timeline.map((e, i) => (
          <div key={i} className={e.body ? "" : "px-3 text-xs text-slate-500 dark:text-slate-400"}>
            {e.body ? (
              <Card>
                <div className="mb-1 text-xs text-slate-500 dark:text-slate-400">{e.actor} {reviewStates[e.state] ?? labels[e.type] ?? e.type} · {new Date(e.at).toLocaleString()}</div>
                <Md>{e.body}</Md>
              </Card>
            ) : <>{e.actor} {reviewStates[e.state] ?? labels[e.type] ?? e.type} {e.detail} · {new Date(e.at).toLocaleString()}</>}
          </div>
        ))}
        <textarea aria-label="Mensaje" value={text} onChange={(e) => setText(e.target.value)} rows={4} placeholder="Escribe un mensaje…" className="w-full rounded border border-slate-300 bg-white p-2 text-sm dark:border-slate-700 dark:bg-slate-900" />
        <div className="flex flex-wrap gap-2">
          <Button disabled={action.isPending || !text.trim()} onClick={() => action.mutate({ type: "comment", body: text })}>Comentar</Button>
          {d.pull && open && <Button variant="ghost" disabled={action.isPending || !text.trim()} onClick={() => action.mutate({ type: "request_changes", body: text })}>Pedir cambios</Button>}
          {d.pull && open && <Button variant="ghost" disabled={action.isPending || !text.trim()} onClick={() => action.mutate({ type: "approve", body: text })}>Aprobar con mensaje</Button>}
        </div>
      </>}
    </div>
  );
}
