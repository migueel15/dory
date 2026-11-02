import { useState } from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { Link } from "@tanstack/react-router";
import { api, type GithubIssue, type GithubPull } from "../api/client";
import { Badge, Button, Card, ErrorText } from "../components/ui";
import { Tip } from "../components/github-ui";

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

const tone = { ok: "bg-green-100 text-green-800 dark:bg-green-950 dark:text-green-300", bad: "bg-red-100 text-red-800 dark:bg-red-950 dark:text-red-300", wait: "bg-amber-100 text-amber-800 dark:bg-amber-950 dark:text-amber-300", none: "bg-slate-100 text-slate-600 dark:bg-slate-800 dark:text-slate-300" };
const toneOf = (color: string) => (color === ok ? tone.ok : color === bad ? tone.bad : color === wait ? tone.wait : tone.none);
const icons = {
  merge: "M6 3v12M18 9a3 3 0 1 0 0-6 3 3 0 0 0 0 6zM6 21a3 3 0 1 0 0-6 3 3 0 0 0 0 6zM18 9a9 9 0 0 1-9 9",
  check: "M20 6 9 17l-5-5",
  ci: "M12 2a10 10 0 1 0 0 20 10 10 0 0 0 0-20zM12 6v6l4 2",
  x: "M18 6 6 18M6 6l12 12",
  eye: "M2 12s3-7 10-7 10 7 10 7-3 7-10 7S2 12 2 12zM12 9a3 3 0 1 0 0 6 3 3 0 0 0 0-6z",
};

function Icon({ d, className = "size-3.5" }: { d: string; className?: string }) {
  return (
    <svg className={className} viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2.5" strokeLinecap="round" strokeLinejoin="round" aria-hidden="true">
      <path d={d} />
    </svg>
  );
}

function Chip({ icon, label, color, title }: { icon: string; label: string; color: string; title: string }) {
  return (
    <Tip text={title}><span className={`inline-flex items-center gap-1 rounded-full px-2 py-0.5 font-medium ${toneOf(color)}`}>
      <Icon d={icon} /> {label}
    </span></Tip>
  );
}

// Avatar shows a GitHub user's picture; the name appears as a tooltip.
export function Avatar({ login, mark, ring, title }: { login: string; mark?: string; ring?: string; title?: string }) {
  return (
    <Tip text={title ?? login}><span className="relative inline-block">
      <img src={`https://github.com/${login}.png?size=48`} alt={login} loading="lazy" className={`size-6 rounded-full bg-slate-200 ring-2 dark:bg-slate-700 ${ring ?? "ring-white dark:ring-slate-900"}`} />
      {mark && <span className={`absolute -bottom-1 -right-1 rounded-full p-px ring-2 ring-white dark:ring-slate-900 ${mark}`}><Icon d={mark.includes("green") ? icons.check : mark.includes("red") ? icons.x : icons.eye} className="size-2.5 text-white" /></span>}
    </span></Tip>
  );
}

export function PullStatus({ pull }: { pull: GithubPull }) {
  const [ci, ciColor] = ciText[pull.ci] ?? [pull.ci, wait];
  const [merge, mergeColor] = mergeStatus(pull);
  const enough = pull.required_approvals ? pull.approvals >= pull.required_approvals : pull.review_decision !== "REVIEW_REQUIRED";
  // One avatar per person. GitHub keeps someone's last review and also lists
  // them as requested when a new review is asked of them; the mark shows the
  // review and an amber ring the pending request.
  const people = new Map<string, { mark?: string; title: string[]; pending: boolean }>();
  const add = (l: string, f: (p: { mark?: string; title: string[]; pending: boolean }) => void) => {
    const p = people.get(l) ?? { title: [], pending: false };
    f(p); people.set(l, p);
  };
  pull.approved_by.forEach((l) => add(l, (p) => { p.mark = "bg-green-600"; p.title.push("aprobó"); }));
  pull.changes_by.forEach((l) => add(l, (p) => { p.mark = "bg-red-600"; p.title.push("pidió cambios"); }));
  pull.review_requests.forEach((l) => add(l, (p) => { p.pending = true; p.title.push(p.mark ? "revisión pedida de nuevo" : "revisión pendiente"); }));
  return (
    <div className="flex flex-wrap items-center gap-2 text-xs">
      <Chip icon={icons.merge} label={merge} color={mergeColor} title="Estado de merge" />
      <Chip icon={icons.check} label={pull.required_approvals ? `${pull.approvals}/${pull.required_approvals}` : `${pull.approvals}`} color={enough ? ok : wait} title={approvalsText(pull)} />
      <Chip icon={pull.ci === "SUCCESS" ? icons.check : pull.ci === "FAILURE" || pull.ci === "ERROR" ? icons.x : icons.ci} label="CI" color={ciColor} title={ci} />
      {people.size > 0 && (
        <span className="ml-1 flex items-center gap-1.5">
          {[...people].map(([l, p]) => (
            <Avatar key={l} login={l} mark={p.mark ?? (p.pending ? "bg-amber-500" : undefined)} ring={p.mark && p.pending ? "ring-amber-500" : undefined} title={`${l}: ${p.title.join(" · ")}`} />
          ))}
        </span>
      )}
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

export const useGithubOpen = () => useQuery({ queryKey: ["github"], queryFn: () => api.github(), refetchInterval: 60_000, retry: false });

export function Github() {
  const qc = useQueryClient();
  const [refreshing, setRefreshing] = useState(false);
  const data = useGithubOpen();
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
