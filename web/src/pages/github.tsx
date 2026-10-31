import { useState } from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { Link } from "@tanstack/react-router";
import { api, type GithubIssue, type GithubPull } from "../api/client";
import { Badge, Button, Card, ErrorText } from "../components/ui";

const ok = "text-green-700 dark:text-green-400";
const bad = "text-red-600 dark:text-red-400";
const wait = "text-amber-700 dark:text-amber-400";
const ciText: Record<string, [string, string]> = { SUCCESS: ["CI correcta", ok], FAILURE: ["CI fallida", bad], ERROR: ["CI con error", bad], PENDING: ["CI en curso", wait], EXPECTED: ["CI pendiente", wait], "": ["Sin CI", "text-slate-500 dark:text-slate-400"] };

// Text and color of the merge status; "BLOCKED" is what branch protection reports.
function mergeStatus(p: GithubPull): [string, string] {
  if (p.draft) return ["Draft", wait];
  if (p.mergeable === "CONFLICTING") return ["Con conflictos", bad];
  if (p.merge_state === "BLOCKED") return ["Bloqueada", bad];
  if (p.merge_state === "BEHIND") return ["Rama desactualizada", wait];
  if (p.mergeable === "MERGEABLE") return ["Se puede mergear", ok];
  return ["Calculando", wait];
}

export function approvalsText(p: GithubPull) {
  return p.required_approvals ? `${p.approvals}/${p.required_approvals} aprobaciones` : `${p.approvals} aprobaciones`;
}

export function PullStatus({ pull }: { pull: GithubPull }) {
  const [ci, ciColor] = ciText[pull.ci] ?? [pull.ci, wait];
  const [merge, mergeColor] = mergeStatus(pull);
  const enough = pull.required_approvals ? pull.approvals >= pull.required_approvals : pull.review_decision !== "REVIEW_REQUIRED";
  return (
    <div className="flex flex-wrap gap-x-3 text-xs">
      <span className={mergeColor}>{merge}</span>
      <span className={enough ? ok : wait}>{approvalsText(pull)}</span>
      <span className={ciColor}>{ci}</span>
      {pull.changes_by.length > 0 && <span className={bad}>Cambios pedidos: {pull.changes_by.join(", ")}</span>}
      {pull.review_requests.length > 0 && <span className="text-slate-500 dark:text-slate-400">Revisión pedida: {pull.review_requests.join(", ")}</span>}
    </div>
  );
}

function Row({ repo, kind, number, title, url, author, children }: { repo: string; kind: string; number: number; title: string; url: string; author: string; children: React.ReactNode }) {
  const [owner, name] = repo.split("/");
  return (
    <Card className="hover:bg-slate-50 dark:hover:bg-slate-800">
      <div className="flex items-start justify-between gap-2">
        <Link to="/github/$owner/$repo/$kind/$number" params={{ owner, repo: name, kind, number: String(number) }} className="min-w-0 flex-1 space-y-1">
          <div className="font-medium break-words">{title}</div>
          <div className="text-xs text-slate-500 dark:text-slate-400">{repo}#{number} · {author}</div>
          {children}
        </Link>
        <a href={url} target="_blank" rel="noreferrer" className="shrink-0 rounded px-2 py-1 text-xs underline">Abrir en GitHub</a>
      </div>
    </Card>
  );
}

const linked = (prefix: string, refs: { number: number; title: string }[]) => refs.length > 0 && (
  <div className="flex flex-wrap gap-1">{refs.map((r) => <span key={r.number} title={r.title}><Badge>{prefix} #{r.number}</Badge></span>)}</div>
);

export function Github() {
  const qc = useQueryClient();
  const [refreshing, setRefreshing] = useState(false);
  const data = useQuery({ queryKey: ["github"], queryFn: () => api.github(), refetchInterval: 60_000, retry: false });
  const refresh = async () => {
    setRefreshing(true);
    try { qc.setQueryData(["github"], await api.github(true)); } finally { setRefreshing(false); }
    qc.invalidateQueries({ queryKey: ["github-detail"] });
  };
  const pulls: GithubPull[] = data.data?.pulls ?? [];
  const issues: GithubIssue[] = data.data?.issues ?? [];
  return (
    <div className="space-y-3">
      <div className="flex items-center justify-between">
        <h1 className="text-xl font-bold">GitHub</h1>
        <Button onClick={refresh} disabled={refreshing}>{refreshing ? "Recargando…" : "Recargar"}</Button>
      </div>
      <ErrorText error={data.error} />
      <div className="grid gap-4 lg:grid-cols-2">
        <section className="min-w-0 space-y-2">
          <h2 className="font-semibold">Pull requests ({pulls.length})</h2>
          {pulls.map((p) => <Row key={p.url} repo={p.repo} kind="pulls" number={p.number} title={p.title} url={p.url} author={p.author}><PullStatus pull={p} />{linked("Issue", p.issues)}</Row>)}
        </section>
        <section className="min-w-0 space-y-2">
          <h2 className="font-semibold">Issues ({issues.length})</h2>
          {issues.map((i) => <Row key={i.url} repo={i.repo} kind="issues" number={i.number} title={i.title} url={i.url} author={i.author}>{linked("PR", i.pulls)}<div className="flex flex-wrap gap-1">{i.labels.map((l) => <Badge key={l}>{l}</Badge>)}</div></Row>)}
        </section>
      </div>
    </div>
  );
}
