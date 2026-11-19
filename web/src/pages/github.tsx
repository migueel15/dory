import { DefineIssueDialog } from "./define-issue";
import { ArrowClockwise, MagnifyingGlass } from "@phosphor-icons/react";
import { useLayoutEffect, useMemo, useRef, useState } from "react";
import { useInfiniteQuery, useQuery, useQueryClient } from "@tanstack/react-query";
import { Link, useNavigate } from "@tanstack/react-router";
import { api, type GithubIssue, type GithubPull } from "../api/client";
import { Arrow, Card, ErrorText, Input, Select, pill } from "../components/ui";
import { GithubMark, Icon, PrRefChip, Tip, icons } from "../components/github-ui";
import { type Icon as PhosphorIcon } from "@phosphor-icons/react";
import { useProject } from "../project";
import { buildStacks, groupByStack, pullKey } from "../stack";

const ok = "text-green-700 dark:text-green-400";
const bad = "text-red-600 dark:text-red-400";
const wait = "text-amber-700 dark:text-amber-400";
const ciText: Record<string, [string, string]> = { SUCCESS: ["CI correcta", ok], FAILURE: ["CI fallida", bad], ERROR: ["CI con error", bad], PENDING: ["CI en curso", wait], EXPECTED: ["CI pendiente", wait], "": ["Sin CI", "text-slate-500 dark:text-muted"] };

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

const tone = { ok: "bg-green-100 text-green-800 dark:bg-green-950 dark:text-green-300", bad: "bg-red-100 text-red-800 dark:bg-red-950 dark:text-red-300", wait: "bg-amber-100 text-amber-800 dark:bg-amber-950 dark:text-amber-300", none: "bg-slate-100 text-slate-600 dark:bg-raised dark:text-text-secondary" };
const toneOf = (color: string) => (color === ok ? tone.ok : color === bad ? tone.bad : color === wait ? tone.wait : tone.none);
function Chip({ icon, label, color, title }: { icon: PhosphorIcon; label: string; color: string; title: string }) {
  return (
    <Tip text={title}><span className={`inline-flex items-center gap-1 rounded-full px-2 py-0.5 font-medium ${toneOf(color)}`}>
      <Icon i={icon} className="size-3.5" /> {label}
    </span></Tip>
  );
}

// Avatar shows a GitHub user's picture; the name appears as a tooltip.
export function Avatar({ login, mark, ring, title }: { login: string; mark?: string; ring?: string; title?: string }) {
  return (
    <Tip text={title ?? login}><span className="relative inline-block">
      <img src={`https://github.com/${login}.png?size=48`} alt={login} loading="lazy" className={`size-6 rounded-full bg-slate-200 ring-2 dark:bg-hover ${ring ?? "ring-white dark:ring-surface"}`} />
      {mark && <span className={`absolute -bottom-1 -right-1 rounded-full p-px ring-2 ring-white dark:ring-surface ${mark}`}><Icon i={mark.includes("green") ? icons.check : mark.includes("red") ? icons.x : icons.eye} className="size-2.5 text-white" /></span>}
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

// ago formats an ISO date as a short relative time in Spanish.
export function ago(iso: string) {
  if (!iso) return "";
  const s = Math.max(0, (Date.now() - new Date(iso).getTime()) / 1000);
  const [n, u] = s < 3600 ? [Math.floor(s / 60), "min"] : s < 86400 ? [Math.floor(s / 3600), "h"] : s < 2592000 ? [Math.floor(s / 86400), "d"] : s < 31536000 ? [Math.floor(s / 2592000), "mes"] : [Math.floor(s / 31536000), "a"];
  return n < 1 ? "ahora" : `hace ${n} ${u}`;
}

const Meta = ({ icon, children, title }: { icon: PhosphorIcon; children: React.ReactNode; title: string }) => (
  <Tip text={title}><span className="inline-flex items-center gap-1"><Icon i={icon} className="size-3.5" />{children}</span></Tip>
);

// Label uses the color GitHub gives it, softened so it reads in both themes.
function Label({ name, color }: { name: string; color?: string }) {
  const c = color ? `#${color}` : undefined;
  return (
    <span className="inline-flex items-center gap-1 rounded-full border px-2 py-0.5 text-[11px] font-medium text-slate-700 dark:text-text" style={c ? { borderColor: `${c}88`, backgroundColor: `${c}22` } : undefined}>
      <span className="size-2 rounded-full" style={{ backgroundColor: c ?? "#94a3b8" }} />{name}
    </span>
  );
}

type CardItem = { repo: string; number: number; title: string; url: string; author: string; created_at: string; updated_at: string; comments: number; assignees: string[]; labels: string[]; label_colors: Record<string, string> };

// Row is the shared card of issues and pull requests: identity, people, activity and labels.
function Row({ item, kind, meta, children }: { item: CardItem; kind: "pulls" | "issues"; meta?: React.ReactNode; children?: React.ReactNode }) {
  const [owner, name] = item.repo.split("/");
  const pull = kind === "pulls";
  const navigate = useNavigate();
  // A click anywhere on the card opens it, except on its own links and buttons.
  const open = (e: React.MouseEvent) => {
    if ((e.target as HTMLElement).closest("a, button") || window.getSelection()?.toString()) return;
    navigate({ to: "/github/$owner/$repo/$kind/$number", params: { owner, repo: name, kind, number: String(item.number) } });
  };
  return (
    <div onClick={open} className="cursor-pointer">
    <Card className="group space-y-2 transition-colors hover:border-slate-300 hover:bg-slate-50/60 dark:hover:border-border-strong dark:hover:bg-raised/40">
      <div className="flex items-start gap-2">
        <Link to="/github/$owner/$repo/$kind/$number" params={{ owner, repo: name, kind, number: String(item.number) }} className="min-w-0 flex-1">
          <div className="font-medium break-words group-hover:text-blue-700 dark:group-hover:text-accent-text">{item.title}</div>
          <div className="mt-0.5 flex items-center gap-1.5 text-xs text-slate-500 dark:text-muted">
            <span title={pull ? "Pull request" : "Issue"} className={`grid size-4.5 shrink-0 place-items-center rounded-full ${pull ? "bg-violet-100 text-violet-700 dark:bg-violet-950 dark:text-violet-300" : "bg-green-100 text-green-700 dark:bg-green-950 dark:text-green-300"}`}>
              <Icon i={pull ? icons.pull : icons.issue} className="size-3" />
            </span>
            <span className="truncate font-mono">{item.repo}</span><span className="font-semibold">#{item.number}</span>
          </div>
        </Link>
        <Tip text="Abrir en GitHub"><a href={item.url} target="_blank" rel="noreferrer" aria-label="Abrir en GitHub" className={pill}><GithubMark className="size-3.5" /></a></Tip>
      </div>
      <div className="flex flex-wrap items-center gap-x-3 gap-y-1 text-xs text-slate-500 dark:text-muted">
        <span className="inline-flex items-center gap-1.5"><Avatar login={item.author} title={`Autor: ${item.author}`} /><span className="font-medium text-slate-700 dark:text-text">{item.author}</span></span>
        <Meta icon={icons.clock} title={`Creada ${new Date(item.created_at).toLocaleString()} · actualizada ${new Date(item.updated_at).toLocaleString()}`}>{ago(item.updated_at)}</Meta>
        <Meta icon={icons.comment} title={`${item.comments} comentarios`}>{item.comments}</Meta>
        {meta}
        {item.assignees.length > 0 && (
          <span className="ml-auto flex items-center -space-x-1.5">{item.assignees.map((a) => <Avatar key={a} login={a} title={`Asignada a ${a}`} />)}</span>
        )}
      </div>
      {(children || item.labels.length > 0) && (
        <div className="flex flex-wrap items-center gap-1.5">
          {children}
          {item.labels.map((l) => <Label key={l} name={l} color={item.label_colors[l]} />)}
        </div>
      )}
    </Card>
    </div>
  );
}

// linked turns the issues or pull requests referenced by a card into links to their own view.
const linked = (repo: string, prefix: "PR" | "Issue", refs: { number: number; title: string }[]) => {
  const [owner, name] = repo.split("/");
  return refs.map((r) => (
    <Tip key={r.number} text={r.title}>
      <Link to="/github/$owner/$repo/$kind/$number" params={{ owner, repo: name, kind: prefix === "PR" ? "pulls" : "issues", number: String(r.number) }}
        className="inline-flex items-center gap-1 rounded-md border border-slate-200 bg-white px-2 py-0.5 text-[11px] font-medium text-slate-700 shadow-sm transition-colors hover:border-blue-400 hover:bg-blue-50 hover:text-blue-700 dark:border-border dark:bg-surface dark:text-text dark:hover:border-accent dark:hover:bg-accent-subtle dark:hover:text-accent-text">
        <Icon i={prefix === "PR" ? icons.pull : icons.issue} className="size-3" />{prefix} #{r.number}
      </Link>
    </Tip>
  ));
};

// The server answers from its cache and refreshes in the background; while it
// does, poll every second so each repository shows up as soon as it is ready.
export const useGithubOpen = () => useQuery({ queryKey: ["github"], queryFn: () => api.github(), refetchInterval: (q) => (q.state.data?.refreshing ? 1_000 : 60_000), retry: false });

type Tab = "pulls" | "issues";
type State = "open" | "closed" | "all";

// ClosedChip shows how a closed item ended, in place of the live status of open ones.
function ClosedChip({ item }: { item: GithubPull | GithubIssue }) {
  const merged = item.state === "MERGED";
  const planned = "state_reason" in item && item.state_reason === "NOT_PLANNED";
  const [label, cls] = merged ? ["Mergeada", "bg-purple-100 text-purple-800 dark:bg-purple-950 dark:text-purple-300"]
    : planned ? ["No planificada", "bg-slate-200 text-slate-700 dark:bg-raised dark:text-text-secondary"]
    : "state_reason" in item ? ["Completada", "bg-purple-100 text-purple-800 dark:bg-purple-950 dark:text-purple-300"]
    : ["Cerrada sin mergear", "bg-red-100 text-red-800 dark:bg-red-950 dark:text-red-300"];
  return <Tip text={item.closed_at ? `Cerrada ${new Date(item.closed_at).toLocaleString()}` : "Cerrada"}><span className={`inline-flex items-center gap-1 rounded-full px-2 py-0.5 text-xs font-medium ${cls}`}>{label}{item.closed_at && ` · ${ago(item.closed_at)}`}</span></Tip>;
}
type Quick = { id: string; label: string; me?: true; test: (x: GithubPull & GithubIssue, me: string) => boolean };

// Quick filters per tab; the ones about "me" need the viewer login.
const quick: Record<Tab, Quick[]> = {
  pulls: [
    { id: "mine", label: "Mías", me: true, test: (p, me) => p.author === me },
    { id: "assigned", label: "Asignadas a mí", me: true, test: (p, me) => p.assignees.includes(me) },
    { id: "review", label: "Revisión pedida a mí", me: true, test: (p, me) => p.review_requests.includes(me) },
    { id: "draft", label: "Drafts", test: (p) => p.draft },
    { id: "unassigned", label: "Sin asignar", test: (p) => p.assignees.length === 0 },
  ],
  issues: [
    { id: "mine", label: "Mías", me: true, test: (i, me) => i.author === me },
    { id: "assigned", label: "Asignadas a mí", me: true, test: (i, me) => i.assignees.includes(me) },
    { id: "unassigned", label: "Sin asignar", test: (i) => i.assignees.length === 0 },
    { id: "nopr", label: "Sin PR", test: (i) => i.pulls.length === 0 },
  ],
};

const savedTab = (): Tab => { try { return localStorage.getItem("github-tab") === "issues" ? "issues" : "pulls"; } catch { return "pulls"; } };

const Skeleton = () => (
  <Card className="animate-pulse space-y-2.5">
    <div className="h-4 w-2/3 rounded bg-slate-200 dark:bg-hover" />
    <div className="h-3 w-1/3 rounded bg-slate-100 dark:bg-raised" />
    <div className="flex gap-2"><div className="size-5 rounded-full bg-slate-200 dark:bg-hover" /><div className="h-3 w-40 rounded bg-slate-100 dark:bg-raised" /></div>
  </Card>
);

function Empty({ filtered, kind, state }: { filtered: boolean; kind: Tab; state: State }) {
  return (
    <div className="flex flex-col items-center gap-2 rounded-xl border border-dashed border-slate-300 px-4 py-10 text-center dark:border-border">
      <span className="grid size-10 place-items-center rounded-full bg-slate-100 text-slate-500 dark:bg-raised dark:text-muted"><Icon i={kind === "pulls" ? icons.pull : icons.issue} className="size-5" /></span>
      <p className="font-medium">{filtered ? "Nada coincide con estos filtros" : `No hay ${kind === "pulls" ? "pull requests" : "issues"}${state === "open" ? " abiertas" : state === "closed" ? " cerradas" : ""}`}</p>
      <p className="text-sm text-slate-500 dark:text-muted">{filtered ? "Prueba con otro texto o quita el filtro rápido." : "Cuando haya alguna en los repos del proyecto aparecerá aquí."}</p>
    </div>
  );
}

const NO_MILESTONE = "\u0000none";

export function Github() {
  const qc = useQueryClient();
  const [resolving, setResolving] = useState<GithubPull | null>(null);
  const [asking, setAsking] = useState(false);
  const [tab, setTabState] = useState<Tab>(savedTab);
  const [text, setText] = useState("");
  const [filter, setFilter] = useState("");
  const [state, setState] = useState<State>("open");
  const [milestone, setMilestone] = useState("");
  const data = useGithubOpen();
  const refreshing = asking || !!data.data?.refreshing;
  const refresh = async () => {
    setAsking(true);
    try { qc.setQueryData(["github"], await api.github(true)); } finally { setAsking(false); }
    qc.invalidateQueries({ queryKey: ["github-detail"] });
  };
  const setTab = (t: Tab) => { setTabState(t); setFilter(""); setMilestone(""); try { localStorage.setItem("github-tab", t); } catch { /* storage unavailable */ } };
  const project = useProject();
  const me = data.data?.viewer ?? "";
  // Closed items are read from GitHub only when asked for, one page per repository at a time.
  const closed = useInfiniteQuery({
    queryKey: ["github-closed", tab, project],
    queryFn: ({ pageParam }) => api.githubClosed(tab, project, pageParam),
    initialPageParam: "",
    getNextPageParam: (last) => last.cursor || undefined,
    enabled: state !== "open",
    staleTime: 60_000,
  });
  const closedPulls = tab === "pulls" ? closed.data?.pages.flatMap((p) => p.pulls) ?? [] : [];
  const closedIssues = tab === "issues" ? closed.data?.pages.flatMap((p) => p.issues) ?? [] : [];
  const byUpdated = <T extends { updated_at: string }>(l: T[]) => [...l].sort((a, b) => b.updated_at.localeCompare(a.updated_at));
  const openPulls: GithubPull[] = (data.data?.pulls ?? []).filter((p) => !project || p.project_id === project);
  const openIssues: GithubIssue[] = (data.data?.issues ?? []).filter((i) => !project || i.project_id === project);
  const pulls = state === "open" ? openPulls : state === "closed" ? byUpdated(closedPulls) : byUpdated([...openPulls, ...closedPulls]);
  const issues = state === "open" ? openIssues : state === "closed" ? byUpdated(closedIssues) : byUpdated([...openIssues, ...closedIssues]);
  const q = text.trim().toLowerCase();
  const matches = (x: GithubPull | GithubIssue) => !q || [x.title, x.repo, `#${x.number}`, x.author, ...x.labels].some((v) => v.toLowerCase().includes(q));
  // NO_MILESTONE keeps the items without one; "" keeps them all.
  const inMilestone = (x: GithubPull | GithubIssue) => !milestone || x.milestone === (milestone === NO_MILESTONE ? "" : milestone);
  const milestones = [...new Set((tab === "pulls" ? pulls : issues).map((x) => x.milestone).filter(Boolean))].sort();
  const milestoneOptions = [{ value: "", label: "Todos los milestones" }, ...milestones.map((m) => ({ value: m, label: m })), { value: NO_MILESTONE, label: "Sin milestone" }];
  // Each list only takes the quick filter of its own tab: an issue has no reviewers, a pull request no linked PRs.
  const keep = (kind: Tab) => {
    const active = tab === kind ? quick[kind].find((f) => f.id === filter) : undefined;
    return (x: GithubPull | GithubIssue) => matches(x) && inMilestone(x) && (!active || active.test(x as GithubPull & GithubIssue, me));
  };
  // Stacks come from every open pull request; a filter may hide some of their members.
  const stacks = useMemo(() => buildStacks(openPulls), [data.data, project]);
  const shownPulls = groupByStack(pulls.filter(keep("pulls")), stacks);
  const shownIssues = issues.filter(keep("issues"));
  const loading = state === "closed" ? closed.isPending : data.isPending || (refreshing && openPulls.length + openIssues.length === 0) || (state === "all" && closed.isPending);
  // Counters show the open ones: closed totals are unknown until every page is read.
  const tabs: [Tab, string, number, PhosphorIcon][] = [["pulls", "Pull requests", openPulls.length, icons.pull], ["issues", "Issues", openIssues.length, icons.issue]];
  const states: [State, string][] = [["open", "Abiertas"], ["closed", "Cerradas"], ["all", "Todas"]];
  const more = state !== "open" && closed.hasNextPage;
  return (
    <div className="space-y-4">
      {resolving && <DefineIssueDialog resolve repo={resolving.repo} number={resolving.number} onClose={() => setResolving(null)} />}
      <div className="flex flex-wrap items-center justify-between gap-3">
        <div className="flex min-w-0 items-baseline gap-2">
          <h1 className="text-xl font-bold">GitHub</h1>
          <span className="truncate text-sm text-slate-500 dark:text-muted">
            {refreshing ? "Actualizando…" : data.data?.fetched_at ? `Actualizado ${ago(data.data.fetched_at)}` : ""}{me && ` · como ${me}`}
          </span>
        </div>
        <Tip text="Recargar desde GitHub">
          <button type="button" onClick={refresh} disabled={refreshing} aria-label="Recargar" className={pill}>
            <ArrowClockwise weight="regular" aria-hidden="true" className={`size-3.5 ${refreshing ? "animate-spin" : ""}`} />
            {refreshing ? "Recargando" : "Recargar"}
          </button>
        </Tip>
      </div>
      <ErrorText error={data.error ?? (state !== "open" ? closed.error : null)} />
      <div className="space-y-3 rounded-xl border border-slate-200 bg-white p-2 shadow-sm dark:border-border dark:bg-surface">
        <div className="flex flex-wrap items-center gap-2">
          <Segmented role="tablist" active={tab}>
            {tabs.map(([id, label, n, icon]) => (
              <button key={id} type="button" role="tab" aria-selected={tab === id} onClick={() => setTab(id)}
                className={`flex min-h-9 items-center gap-1.5 relative z-10 rounded-md px-3 text-sm font-medium transition-colors ${tab === id ? "text-slate-900 dark:text-white" : "text-slate-500 hover:text-slate-800 dark:text-muted dark:hover:text-text"}`}>
                <Icon i={icon} className="size-3.5" />{label}
                <span className={`rounded-full px-1.5 text-xs ${tab === id ? "bg-slate-100 dark:bg-raised" : "bg-slate-200 dark:bg-hover"}`}>{n}</span>
              </button>
            ))}
          </Segmented>
          <Segmented role="radiogroup" aria-label="Estado" active={state}>
            {states.map(([id, label]) => (
              <button key={id} type="button" role="radio" aria-checked={state === id} onClick={() => setState(id)}
                className={`relative z-10 min-h-9 rounded-md px-3 text-sm font-medium transition-colors ${state === id ? "text-slate-900 dark:text-white" : "text-slate-500 hover:text-slate-800 dark:text-muted dark:hover:text-text"}`}>
                {label}
              </button>
            ))}
          </Segmented>
          <label className="relative min-w-48 flex-1">
            <MagnifyingGlass weight="regular" aria-hidden="true" className="pointer-events-none absolute left-2.5 top-1/2 size-4 -translate-y-1/2 text-slate-400" />
            <Input value={text} onChange={(e) => setText(e.target.value)} placeholder="Buscar por título, repo, autor o etiqueta…" aria-label="Buscar"
              className="min-h-9 pl-8" />
          </label>
          <Select value={milestone} onChange={setMilestone} options={milestoneOptions} className="min-w-44" />
        </div>
        <div className="flex flex-wrap gap-1.5">
          {quick[tab].map((f) => {
            const on = filter === f.id;
            const needsMe = !!f.me && !me;
            return (
              <button key={f.id} type="button" disabled={needsMe} onClick={() => setFilter(on ? "" : f.id)}
                className={`rounded-md border px-2.5 py-1 text-xs font-medium transition-colors disabled:opacity-40 ${on ? "border-blue-500 bg-blue-50 text-blue-700 dark:border-accent dark:bg-accent-subtle dark:text-accent-text" : "border-slate-200 text-slate-600 hover:border-slate-300 hover:bg-slate-50 dark:border-border dark:text-text-secondary dark:hover:bg-raised"}`}>
                {f.label}
              </button>
            );
          })}
        </div>
      </div>
      <section className="space-y-2">
        {loading ? [0, 1, 2].map((n) => <Skeleton key={n} />)
          : tab === "pulls" ? (shownPulls.length === 0 ? <Empty kind="pulls" state={state} filtered={pulls.length > 0} /> : shownPulls.map(({ pull: p, depth, label }) => (
            <div key={p.url} style={depth ? { marginLeft: `${Math.min(depth, 4) * 1.25}rem` } : undefined} className={depth ? "border-l-2 border-violet-300 pl-2 dark:border-violet-800" : undefined}>
            <Row item={p} kind="pulls" meta={<>
              {label && <Tip text="Pull requests encadenadas: la base de cada una es la rama de la anterior"><span className="rounded-full bg-violet-100 px-2 py-0.5 font-medium text-violet-700 dark:bg-violet-950 dark:text-violet-300">{label}</span></Tip>}
              <Meta icon={icons.branch} title={`${p.head_ref} → ${p.base_ref}`}><span className="max-w-40 truncate font-mono">{p.head_ref}</span></Meta>
              <Meta icon={icons.file} title={`${p.changed_files} ficheros cambiados`}>{p.changed_files}</Meta>
              <Tip text="Líneas añadidas y borradas"><span className="font-mono"><span className="text-green-700 dark:text-green-400">+{p.additions}</span> <span className="text-red-600 dark:text-red-400">−{p.deletions}</span></span></Tip>
              {p.milestone && <Meta icon={icons.flag} title="Milestone">{p.milestone}</Meta>}
            </>}>
              {p.state === "OPEN" ? <PullStatus pull={p} /> : <ClosedChip item={p} />}{linked(p.repo, "Issue", p.issues)}
              {p.state === "OPEN" && p.mergeable === "CONFLICTING" && <button type="button" className={pill} onClick={(e) => { e.stopPropagation(); setResolving(p); }}>Resolver conflictos</button>}
            </Row>
            </div>
          )))
          : (shownIssues.length === 0 ? <Empty kind="issues" state={state} filtered={issues.length > 0} /> : shownIssues.map((i) => (
            <Row key={i.url} item={i} kind="issues" meta={i.milestone && <Meta icon={icons.flag} title="Milestone">{i.milestone}</Meta>}>
              {i.state !== "OPEN" && <ClosedChip item={i} />}
              {i.pulls.length > 0 && linked(i.repo, "PR", i.pulls)}
            </Row>
          )))}
        {more && !loading && (
          <div className="flex justify-center pt-1">
            <button type="button" className={pill} disabled={closed.isFetchingNextPage} onClick={() => closed.fetchNextPage()}>
              {closed.isFetchingNextPage ? "Cargando…" : "Cargar más cerradas"}
            </button>
          </div>
        )}
      </section>
    </div>
  );
}

// Segmented is a pill group whose white background slides to the selected option.
function Segmented({ active, children, ...rest }: { active: string; children: React.ReactNode; role: string; "aria-label"?: string }) {
  const box = useRef<HTMLDivElement>(null);
  const [pill, setPill] = useState<{ left: number; width: number } | null>(null);
  useLayoutEffect(() => {
    const place = () => {
      const el = box.current?.querySelector<HTMLElement>("[aria-selected=true],[aria-checked=true]");
      setPill(el ? { left: el.offsetLeft, width: el.offsetWidth } : null);
    };
    place();
    const ro = new ResizeObserver(place);
    if (box.current) ro.observe(box.current);
    return () => ro.disconnect();
  }, [active]);
  return (
    <div ref={box} {...rest} className="relative flex rounded-lg bg-slate-100 p-1 dark:bg-raised">
      {pill && <span aria-hidden="true" style={{ left: pill.left, width: pill.width }} className="absolute inset-y-1 rounded-md bg-white shadow-sm transition-all duration-200 ease-out motion-reduce:transition-none dark:bg-canvas" />}
      {children}
    </div>
  );
}

// StackBar shows the stack a pull request belongs to, from base to top, with
// arrows to its neighbours. It reads the cached overview, so it appears once that loads.
export function StackBar({ repo, number }: { repo: string; number: number }) {
  const data = useGithubOpen();
  const stacks = useMemo(() => buildStacks(data.data?.pulls ?? []), [data.data]);
  const stack = stacks.get(`${repo}#${number}`);
  const navigate = useNavigate();
  if (!stack) return null;
  const [owner, name] = repo.split("/");
  const at = stack.nodes.findIndex((n) => pullKey(n.pull) === `${repo}#${number}`);
  const go = (i: number) => navigate({ to: "/github/$owner/$repo/$kind/$number", params: { owner, repo: name, kind: "pulls", number: String(stack.nodes[i].pull.number) } });
  const arrow = "grid size-7 place-items-center rounded-lg text-slate-500 transition-colors hover:bg-slate-100 disabled:opacity-30 disabled:hover:bg-transparent dark:text-muted dark:hover:bg-raised";
  return (
    <nav aria-label="Stack de pull requests" className="flex flex-wrap items-center gap-1 rounded-lg border border-slate-200 bg-white px-2 py-1.5 text-sm shadow-sm dark:border-border dark:bg-surface">
      <Tip text="Anterior del stack"><button type="button" aria-label="Anterior del stack" disabled={at <= 0} onClick={() => go(at - 1)} className={arrow}><Arrow back /></button></Tip>
      <Tip text="Siguiente del stack"><button type="button" aria-label="Siguiente del stack" disabled={at >= stack.nodes.length - 1} onClick={() => go(at + 1)} className={arrow}><Arrow /></button></Tip>
      <span className="mr-1 font-medium text-slate-500 dark:text-muted">Stack</span>
      {stack.nodes.map(({ pull: p }, i) => (
        <span key={p.number} className="inline-flex items-center gap-1">
          {i > 0 && <span aria-hidden="true" className="text-slate-400"><Arrow /></span>}
          <PrRefChip owner={owner} repo={name} number={p.number} title={p.title} state={p.state} current={i === at} className={i === at ? "bg-slate-100 dark:bg-raised" : ""} />
        </span>
      ))}
    </nav>
  );
}
