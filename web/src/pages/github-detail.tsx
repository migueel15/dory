import { useEffect, useRef, useState } from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { Link, useParams } from "@tanstack/react-router";
import { api, type GithubDetail, type GithubPull, type GithubThread } from "../api/client";
import { Badge, Button, Card, Input, Select, ErrorText, Md, Arrow, pill, toggleTask, Textarea, keepAboveKeyboard } from "../components/ui";
import { Avatar, PullStatus } from "./github";
import { Diff } from "./github-diff";
import { DefineIssueDialog } from "./define-issue";
import { GithubMark, Icon, IconButton, Menu, MenuItem, RefContext, RefMd, Tip, UserPicker, icons, useArm, MentionArea } from "../components/github-ui";

const labels: Record<string, string> = {
  IssueComment: "comentó", PullRequestReview: "revisó", PullRequestCommit: "hizo un commit", ClosedEvent: "cerró", ReopenedEvent: "reabrió", MergedEvent: "mergeó", LabeledEvent: "añadió la etiqueta", AssignedEvent: "asignó", ReadyForReviewEvent: "marcó como lista", ConvertToDraftEvent: "pasó a draft", ReviewRequestedEvent: "pidió revisión", CrossReferencedEvent: "referenció desde",
};
const reviewStates: Record<string, string> = { APPROVED: "aprobó", CHANGES_REQUESTED: "pidió cambios", COMMENTED: "comentó en la revisión", DISMISSED: "revisión descartada" };

// canApprove is false for the author (GitHub rejects it) and for whoever
// already approved; requesting changes again brings the button back.
const canApprove = (pull: GithubPull, me: string) => pull.author !== me && !pull.approved_by.includes(me);

type Act = (type: string, extra?: { method?: string; user_ids?: string[]; title?: string; body?: string; milestone_id?: string }) => void;

// ReviewPicker asks someone for a review; it sits next to the reviewers' avatars.
function ReviewPicker({ d, act, busy }: { d: GithubDetail; act: Act; busy: boolean }) {
  const pull = d.pull!;
  const reviewed = new Set([...pull.approved_by, ...pull.changes_by]);
  const users = d.candidates.filter((u) => u.login !== pull.author && !pull.review_requests.includes(u.login));
  return <UserPicker tip="Pedir revisión" title="Pedir revisión a" users={users} disabled={busy} note={(u) => (reviewed.has(u.login) ? "de nuevo" : undefined)} onPick={(u) => act("request_review", { user_ids: [u.id] })} />;
}

// Assignees shows who owns the issue or pull request and lets Dory's account change it.
function Assignees({ d, act, busy }: { d: GithubDetail; act: Act; busy: boolean }) {
  const assignable = d.candidates.filter((u) => !d.assignees.some((a) => a.id === u.id));
  const me = assignable.find((u) => u.login === d.viewer);
  return (
    <div className="flex items-center gap-2">
      <Tip text="Asignados"><span className="text-slate-400"><Icon i={icons.user} /></span></Tip>
      <span className="flex -space-x-1.5">
        {d.assignees.map((a) => (
          <Tip key={a.id} text={`${a.login} · clic para quitar`}>
            <button type="button" aria-label={`Quitar a ${a.login}`} disabled={busy} onClick={() => act("unassign", { user_ids: [a.id] })} className="group relative rounded-full">
              <img src={`https://github.com/${a.login}.png?size=48`} alt={a.login} className="size-6 rounded-full bg-slate-200 ring-2 ring-white dark:bg-hover dark:ring-surface" />
              <span className="absolute inset-0 grid place-items-center rounded-full bg-red-600/80 text-white opacity-0 transition-opacity group-hover:opacity-100"><Icon i={icons.x} className="size-3" /></span>
            </button>
          </Tip>
        ))}
      </span>
      <UserPicker tip="Asignar" title="Asignar a" users={assignable.filter((u) => u.id !== me?.id)} disabled={busy} onPick={(u) => act("assign", { user_ids: [u.id] })}
        extra={me && ((close) => (
          <MenuItem onClick={() => { close(); act("assign", { user_ids: [me.id] }); }}>
            <span className="grid size-6 place-items-center rounded-full bg-blue-100 text-blue-700 dark:bg-accent-subtle dark:text-accent-text"><Icon i={icons.user} className="size-3.5" /></span>
            <span className="flex-1 font-medium">Asignármela</span>
          </MenuItem>
        ))} />
    </div>
  );
}

// MilestonePicker sets or removes the issue's milestone; a closed one stays listed while it is the current one.
function MilestonePicker({ d, act, busy }: { d: GithubDetail; act: Act; busy: boolean }) {
  const current = d.issue!.milestone;
  const known = d.milestones.find((m) => m.title === current);
  const options = [{ value: "", label: "Sin milestone" }, ...d.milestones.map((m) => ({ value: m.id, label: m.title }))];
  if (current && !known) options.push({ value: "current", label: current });
  const value = known?.id ?? (current ? "current" : "");
  return (
    <span className="inline-flex items-center gap-1.5">
      <Tip text="Milestone"><span className="text-slate-400"><Icon i={icons.flag} /></span></Tip>
      <Select value={value} disabled={busy} options={options} className="min-w-40" onChange={(v) => v !== value && v !== "current" && act("set_milestone", { milestone_id: v })} />
    </span>
  );
}

const methods = [
  { value: "MERGE", name: "Merge commit", hint: "Conserva todos los commits" },
  { value: "SQUASH", name: "Squash", hint: "Un único commit" },
  { value: "REBASE", name: "Rebase", hint: "Commits sobre la base, sin merge" },
];

// MergeButton is a split button: the main part merges with the chosen method.
function MergeButton({ pull, act, busy }: { pull: GithubPull; act: Act; busy: boolean }) {
  const [method, setMethod] = useState(methods[0]);
  const [armed, merge] = useArm(() => act("merge", { method: method.value }));
  return (<>
    <Menu align="right" trigger={(open, toggle) => (
      <div className="inline-flex overflow-hidden rounded-lg bg-green-600 text-white shadow-sm">
        <button type="button" disabled={busy} onClick={merge} className="flex items-center gap-1.5 px-3 py-2 text-sm font-medium hover:bg-green-700 disabled:opacity-50">
          <Icon i={icons.merge} /> {method.name}<span aria-hidden={!armed} className={`overflow-hidden whitespace-nowrap transition-all duration-200 ease-out motion-reduce:transition-none ${armed ? "max-w-28 opacity-100" : "max-w-0 opacity-0"}`}>· ¿Confirmar?</span>
        </button>
        <button type="button" aria-label="Elegir método de merge" disabled={busy} onClick={toggle} className={`border-l border-green-500 px-2 hover:bg-green-700 ${open ? "bg-green-700" : ""}`}>
          <Icon i={icons.chevron} />
        </button>
      </div>
    )}>
      {(close) => methods.map((m) => (
        <MenuItem key={m.value} onClick={() => { setMethod(m); close(); }}>
          <span className="grid size-5 place-items-center text-green-600">{m.value === method.value && <Icon i={icons.check} />}</span>
          <span><span className="block font-medium">{m.name}</span><span className="block text-xs text-slate-500">{m.hint}</span></span>
        </MenuItem>
      ))}
    </Menu></>
  );
}

function PullActions({ pull, me, act, busy, diff, toggleDiff }: { pull: GithubPull; me: string; act: Act; busy: boolean; diff: boolean; toggleDiff: () => void }) {
  const view = <IconButton icon={icons.code} tip={diff ? "Ocultar diff" : "Ver diff"} tone={diff ? "active" : "plain"} onClick={toggleDiff} />;
  if (pull.state !== "OPEN") return <>{view}{pull.state === "CLOSED" && <IconButton icon={icons.reopen} tip="Reabrir" disabled={busy} confirm onClick={() => act("reopen_pull")} />}</>;
  return (
    <>
      {view}
      {pull.draft
        ? <IconButton icon={icons.ready} tip="Marcar como lista para revisión" disabled={busy} confirm onClick={() => act("ready")} />
        : <IconButton icon={icons.draft} tip="Pasar a draft" disabled={busy} confirm onClick={() => act("draft")} />}
      {canApprove(pull, me) && <IconButton icon={icons.check} tip="Aprobar" tone="green" disabled={busy} onClick={() => act("approve")} />}
      <IconButton icon={icons.x} tip="Cerrar sin mergear" tone="red" disabled={busy} confirm onClick={() => act("close_pull")} />
      <span className="ml-auto"><MergeButton pull={pull} act={act} busy={busy} /></span>
    </>
  );
}

// IssueActions closes an issue as completed or not planned, or reopens it.
function IssueActions({ open, act, busy }: { open: boolean; act: Act; busy: boolean }) {
  if (!open) return <><IconButton icon={icons.reopen} tip="Reabrir issue" disabled={busy} confirm onClick={() => act("reopen_issue")} /></>;
  return (
    <>
      <IconButton icon={icons.ready} tip="Cerrar como completada" tone="green" disabled={busy} confirm onClick={() => act("close_issue", { method: "COMPLETED" })} />
      <IconButton icon={icons.x} tip="Cerrar como no planificada" tone="red" disabled={busy} confirm onClick={() => act("close_issue", { method: "NOT_PLANNED" })} />
    </>
  );
}

type ThreadAct = (a: { type: string; thread_id?: string; body?: string; comment_id?: string }) => Promise<unknown>;

// quote turns a message into a markdown quote to answer it.
const quote = (who: string, body: string) => `> **@${who}**\n${body.trim().split("\n").map((l) => `> ${l}`).join("\n")}\n\n`;

// Message is one comment: who and when, the text, and buttons to answer it
// or, when Dory's account wrote it, to edit it in place.
function Message({ author, verb, at, url, body, mine, editType, id, act, busy, onReply }: { author: string; verb?: string; at: string; url?: string; body: string; mine: boolean; editType: string; id: string; act: ThreadAct; busy: boolean; onReply: (q: string) => void }) {
  const [draft, setDraft] = useState<string>();
  const when = new Date(at).toLocaleString();
  const small = "grid size-7 place-items-center rounded-md text-slate-400 transition-colors hover:bg-slate-100 hover:text-slate-700 dark:hover:bg-raised dark:hover:text-text";
  return (
    <div className="group/msg">
      <div className="mb-1 flex items-center gap-2 text-xs text-slate-500 dark:text-muted">
        <Avatar login={author} />
        <span><span className="font-medium text-slate-700 dark:text-text">{author}</span>{verb && ` ${verb}`} · {url ? <a href={url} target="_blank" rel="noreferrer" className="hover:underline">{when}</a> : when}</span>
        <span className="ml-auto flex gap-0.5 opacity-60 transition-opacity group-hover/msg:opacity-100">
          {body && <Tip text="Responder citando"><button type="button" aria-label="Responder citando" onClick={() => onReply(quote(author, body))} className={small}><Icon i={icons.reply} /></button></Tip>}
          {mine && id && !id.startsWith("pending-") && draft === undefined && <Tip text="Editar"><button type="button" aria-label="Editar" onClick={() => setDraft(body)} className={small}><Icon i={icons.edit} /></button></Tip>}
        </span>
      </div>
      {draft === undefined ? <RefMd onToggle={mine && id && !id.startsWith("pending-") && !busy ? (line) => act({ type: editType, comment_id: id, body: toggleTask(body, line) }) : undefined}>{body}</RefMd> : (
        <div className="space-y-2">
          <Textarea aria-label="Editar mensaje" autoFocus value={draft} onChange={(e) => setDraft(e.target.value)} onKeyDown={(e) => e.key === "Escape" && setDraft(undefined)} rows={Math.min(14, draft.split("\n").length + 2)} className="p-2 font-mono" />
          <div className="flex justify-end gap-2">
            <Button variant="ghost" onClick={() => setDraft(undefined)}>Cancelar</Button>
            <Button disabled={busy || !draft.trim()} onClick={() => { act({ type: editType, comment_id: id, body: draft }).catch(() => {}); setDraft(undefined); }}>Guardar</Button>
          </div>
        </div>
      )}
    </div>
  );
}

function ThreadCard({ t, me, people, links, act, busy }: { t: GithubThread; me: string; people: string[]; links: GithubDetail["links"]; act: ThreadAct; busy: boolean }) {
  const [open, setOpen] = useState(!t.resolved);
  const [reply, setReply] = useState("");
  const send = () => { act({ type: "reply_thread", thread_id: t.id, body: reply }).catch(() => {}); setReply(""); };
  return (
    <Card>
      <div className="flex flex-wrap items-center justify-between gap-2">
        <button type="button" onClick={() => setOpen(!open)} className="min-w-0 break-all text-left font-mono text-xs">
          {open ? "▾" : "▸"} {t.path}{t.line ? `:${t.line}` : ""}
        </button>
        <div className="flex flex-wrap items-center gap-1">
          {t.outdated && <Badge>desactualizado</Badge>}
          {t.resolved && <Badge>resuelto{t.resolved_by && ` por ${t.resolved_by}`}</Badge>}
          <Button variant="ghost" disabled={busy} onClick={() => act({ type: t.resolved ? "unresolve_thread" : "resolve_thread", thread_id: t.id })}>
            {t.resolved ? "Desresolver" : "Resolver"}
          </Button>
        </div>
      </div>
      {!open && t.comments[0] && <div className="mt-1 truncate text-xs text-slate-500 dark:text-muted">{t.comments[0].author}: {t.comments[0].body}</div>}
      {open && <>
        {t.comments.map((c, i) => (
          <div key={i} className="mt-2 border-l-2 border-slate-200 pl-2 dark:border-border">
            <Message author={c.author} at={c.at} url={c.url} body={c.body} mine={c.author === me} editType="edit_thread_comment" id={c.id} act={act} busy={busy} onReply={(q) => setReply(q + reply)} />
          </div>
        ))}
        <MentionArea people={people} links={links} aria-label="Responder en el hilo" value={reply} onValue={setReply} rows={2} placeholder="Responder…" className="mt-2 p-2" />
        <div className="flex gap-2">
          <Button disabled={busy || !reply.trim()} onClick={send}>Responder</Button>
          {reply && <Button variant="ghost" onClick={() => setReply("")}>Descartar</Button>}
        </div>
      </>}
    </Card>
  );
}

function Threads({ threads, me, people, links, act, busy }: { threads: GithubThread[]; me: string; people: string[]; links: GithubDetail["links"]; act: ThreadAct; busy: boolean }) {
  if (threads.length === 0) return null;
  const pending = threads.filter((t) => !t.resolved).length;
  return (
    <section className="space-y-2">
      <h2 className="text-sm font-semibold">Conversaciones de revisión · {pending} sin resolver de {threads.length}</h2>
      {threads.map((t) => <ThreadCard key={t.id} t={t} me={me} people={people} links={links} act={act} busy={busy} />)}
    </section>
  );
}

// Header is the single card of the pull request or issue: what it is on top,
// its review state and people in the middle, and the actions at the bottom.
function Header({ d, actions, act, busy, people }: { d: GithubDetail; actions: React.ReactNode; act: Act; busy: boolean; people: string[] }) {
  const e = d.pull ?? d.issue!;
  const live = d.pull?.state === "OPEN";
  const refs = d.pull?.issues ?? d.issue?.pulls ?? [];
  const [owner, repo] = e.repo.split("/");
  const [draft, setDraft] = useState<{ title: string; body: string }>();
  const save = () => { if (draft) { act(d.pull ? "edit_pull" : "edit_issue", draft); setDraft(undefined); } };
  const box = "py-1";
  return (
    <Card className="p-0">
      <div className="space-y-2 p-4">
        <div className="flex items-start gap-3">
          <span className={`mt-0.5 shrink-0 rounded-full px-2.5 py-0.5 text-xs font-semibold text-white ${stateColor[e.state] ?? "bg-slate-500"}`}>{stateName[e.state] ?? e.state}</span>
          {draft
            ? <Input aria-label="Título" autoFocus value={draft.title} onChange={(ev) => setDraft({ ...draft, title: ev.target.value })} onKeyDown={(ev) => ev.key === "Escape" && setDraft(undefined)} className={`${box} min-w-0 flex-1 !text-lg font-bold`} />
            : <h1 className="min-w-0 flex-1 break-words text-lg font-bold leading-snug">{e.title}</h1>}
          {(draft ? (
            <span className="flex shrink-0 gap-1">
              <Tip text="Guardar"><button type="button" aria-label="Guardar" disabled={busy || !draft.title.trim()} onClick={save} className="grid size-8 place-items-center rounded-lg bg-green-600 text-white hover:bg-green-700 disabled:opacity-50"><Icon i={icons.check} /></button></Tip>
              <Tip text="Cancelar"><button type="button" aria-label="Cancelar" onClick={() => setDraft(undefined)} className="grid size-8 place-items-center rounded-lg text-slate-500 hover:bg-slate-100 dark:hover:bg-raised"><Icon i={icons.x} /></button></Tip>
            </span>
          ) : (
            <Tip text="Editar título y descripción"><button type="button" aria-label="Editar título y descripción" onClick={() => setDraft({ title: e.title, body: d.body })} className="grid size-8 shrink-0 place-items-center rounded-lg text-slate-500 transition-colors hover:bg-slate-100 hover:text-slate-900 dark:text-muted dark:hover:bg-raised dark:hover:text-white"><Icon i={icons.edit} /></button></Tip>
          ))}
          <Tip text="Abrir en GitHub"><a href={e.url} target="_blank" rel="noreferrer" aria-label="Abrir en GitHub" className="grid size-8 shrink-0 place-items-center rounded-lg text-slate-500 transition-colors hover:bg-slate-100 hover:text-slate-900 dark:text-muted dark:hover:bg-raised dark:hover:text-white"><GithubMark /></a></Tip>
        </div>
        <div className="flex flex-wrap items-center gap-x-3 gap-y-1 text-xs text-slate-500 dark:text-muted">
          <Avatar login={e.author} title={`Autor: ${e.author}`} />
          <span>{e.repo}#{e.number}</span>
          {d.pull && <span className="rounded bg-slate-100 px-1.5 py-0.5 font-mono dark:bg-raised">{d.pull.head_ref} → {d.pull.base_ref}</span>}
          {e.labels.map((l) => <Badge key={l}>{l}</Badge>)}
          {d.parent && <span className="inline-flex items-center gap-1">Padre: <SubLink s={d.parent} here={e.repo} /></span>}
          {refs.map((r) => <Link key={r.number} className="underline" to="/github/$owner/$repo/$kind/$number" params={{ owner, repo, kind: d.pull ? "issues" : "pulls", number: String(r.number) }}>#{r.number} {r.title}</Link>)}
        </div>
      </div>
      {d.pull && (
        <div className="flex flex-wrap items-center gap-x-6 gap-y-3 border-t border-slate-100 px-4 py-3 dark:border-border">
          <div className="flex items-center gap-2"><PullStatus pull={d.pull} />{live && <ReviewPicker d={d} act={act} busy={busy} />}</div>
          {live && <Assignees d={d} act={act} busy={busy} />}
        </div>
      )}
      {d.issue && (
        <div className="flex flex-wrap items-center gap-x-6 gap-y-3 border-t border-slate-100 px-4 py-3 text-xs text-slate-500 dark:border-border dark:text-muted">
          {d.issue.state === "OPEN" ? <Assignees d={d} act={act} busy={busy} /> : d.assignees.length > 0 && (
            <span className="flex items-center gap-2"><Icon i={icons.user} />{d.assignees.map((a) => <Avatar key={a.id} login={a.login} />)}</span>
          )}
          <MilestonePicker d={d} act={act} busy={busy} />
          <span className="inline-flex items-center gap-1.5"><Icon i={icons.comment} />{d.issue.comments} comentarios</span>
          {d.issue.created_at && <span>Abierta {new Date(d.issue.created_at).toLocaleDateString()}</span>}
        </div>
      )}
      {d.sub_issues.length > 0 && (
        <div className="border-t border-slate-100 px-4 py-3 text-sm dark:border-border">
          <div className="mb-1.5 text-xs font-semibold text-slate-500 dark:text-muted">Sub-issues · {d.sub_issues.filter((s) => s.state !== "OPEN").length}/{d.sub_issues.length} cerradas</div>
          <ul className="space-y-1">{d.sub_issues.map((s) => <li key={`${s.repo}#${s.number}`}><SubLink s={s} here={e.repo} dot /></li>)}</ul>
        </div>
      )}
      {draft
        ? <div className="border-t border-slate-100 px-4 py-3 dark:border-border"><MentionArea aria-label="Descripción" value={draft.body} onValue={(body) => setDraft({ ...draft, body })} people={people} links={d.links} rows={12} className={`${box} font-mono`} /></div>
        : d.body && <div className="max-h-[60vh] overflow-auto border-t border-slate-100 px-4 py-3 dark:border-border"><RefMd onToggle={busy ? undefined : (line) => act(d.pull ? "edit_pull" : "edit_issue", { title: e.title, body: toggleTask(d.body, line) })}>{d.body}</RefMd></div>}
      <div className="flex flex-wrap items-center gap-2 rounded-b-lg border-t border-slate-100 bg-slate-50 px-4 py-2.5 dark:border-border dark:bg-canvas/40">{actions}</div>
    </Card>
  );
}

// SubLink opens a parent or sub-issue inside Dory, even in another repository.
function SubLink({ s, here, dot }: { s: GithubDetail["sub_issues"][number]; here: string; dot?: boolean }) {
  const [owner, repo] = s.repo.split("/");
  return (
    <Link className="inline-flex items-center gap-1.5 underline" to="/github/$owner/$repo/$kind/$number" params={{ owner, repo, kind: "issues", number: String(s.number) }}>
      {dot && <span className={`size-2 shrink-0 rounded-full ${stateColor[s.state] ?? "bg-slate-500"}`} />}
      {s.repo === here ? "" : s.repo}#{s.number} {s.title}
    </Link>
  );
}

const stateColor: Record<string, string> = { OPEN: "bg-green-600", CLOSED: "bg-red-600", MERGED: "bg-purple-600" };
const stateName: Record<string, string> = { OPEN: "Abierta", CLOSED: "Cerrada", MERGED: "Mergeada" };

export function GithubView() {
  const { owner, repo, kind, number } = useParams({ strict: false }) as Record<string, string>;
  const full = `${owner}/${repo}`;
  const n = Number(number);
  const qc = useQueryClient();
  const [text, setText] = useState("");
  const box = useRef<HTMLTextAreaElement>(null);
  const discard = () => { setText(""); box.current?.blur(); };
  // On phones the keyboard shrinks the visual viewport after focus; keep the box above it.
  const focusBox = () => requestAnimationFrame(() => {
    const el = box.current; if (!el) return;
    el.focus({ preventScroll: true });
    keepAboveKeyboard(bottom.current);
  });
  const write = () => { bottom.current?.scrollIntoView({ behavior: "smooth", block: "end" }); focusBox(); };
  const [showDiff, setShowDiff] = useState(false);
  const [defining, setDefining] = useState(false);
  const key = ["github-detail", full, kind, n];
  const detail = useQuery({ queryKey: key, queryFn: () => api.githubDetail(full, kind, n), refetchInterval: 60_000 });
  // The floating button hides while the composer area is on screen.
  const bottom = useRef<HTMLDivElement>(null);
  const [atBottom, setAtBottom] = useState(false);
  useEffect(() => {
    const el = bottom.current;
    if (!el) return;
    const io = new IntersectionObserver(([e]) => setAtBottom(e.isIntersecting));
    io.observe(el);
    return () => io.disconnect();
  }, [detail.isSuccess]);
  const action = useActionQueue(key, () => {});
  const d = detail.data;
  // People who can be mentioned: the repo's users and everyone in the conversation.
  const people = d ? [...new Set([...d.candidates.map((u) => u.login), (d.pull ?? d.issue)!.author, ...d.timeline.map((e) => e.actor), ...d.threads.flatMap((t) => t.comments.map((c) => c.author))])].filter((p) => /^[\w-]+$/.test(p) && p !== d.viewer).sort() : [];
  const act: Act = (type, extra = {}) => action.mutate({ type, ...extra });
  const open = (d?.pull ?? d?.issue)?.state === "OPEN";
  return (
    <div className="space-y-3">
      <Link to="/github" className={pill}><Arrow back /> GitHub</Link>
      <ErrorText error={detail.error ?? action.error} />
      {detail.isPending && <DetailSkeleton />}
      {d && <RefContext.Provider value={{ owner: full.split("/")[0], repo: full.split("/")[1], links: d.links }}>
        <Header d={d} act={act} busy={false} people={people} actions={<>
          {d.pull && <PullActions pull={d.pull} me={d.viewer} act={act} busy={false} diff={showDiff} toggleDiff={() => setShowDiff(!showDiff)} />}
          {d.pull?.mergeable === "CONFLICTING" && open && <Button variant="ghost" className="text-xs" onClick={() => setDefining(true)}>Resolver conflictos</Button>}
          {d.issue && open && <Button variant="ghost" className="text-xs" onClick={() => setDefining(true)}>Definir tareas</Button>}
          {d.issue && <IssueActions open={open} act={act} busy={false} />}
        </>} />
        {defining && <DefineIssueDialog resolve={!!d.pull} repo={full} number={n} onClose={() => setDefining(false)} />}
        {showDiff && <Diff repo={full} pr={n} />}
        <Threads threads={d.threads} me={d.viewer} people={people} links={d.links} act={action.mutateAsync} busy={false} />
        {d.timeline.map((e, i) => (
          <div key={i} className={e.body ? "" : "px-3 text-xs text-slate-500 dark:text-muted"}>
            {e.body ? (
              <Card>
                <Message author={e.actor} verb={reviewStates[e.state] ?? labels[e.type] ?? e.type} at={e.at} body={e.body} mine={e.actor === d.viewer} editType={e.type === "PullRequestReview" ? "edit_review" : "edit_comment"} id={e.id} act={action.mutateAsync} busy={false}
                  onReply={(q) => { setText(q + text); write(); }} />
              </Card>
            ) : <>{e.actor} {reviewStates[e.state] ?? labels[e.type] ?? e.type} {e.detail} · {new Date(e.at).toLocaleString()}</>}
          </div>
        ))}
        <div ref={bottom}>
          <Card className="space-y-2">
            <MentionArea boxRef={box} people={people} links={d.links} id="composer" aria-label="Mensaje" value={text} onValue={setText} onKeyDown={(e) => e.key === "Escape" && box.current?.blur()} rows={4} placeholder="Escribe un mensaje…" className="p-2" />
            <div className="flex flex-wrap gap-2">
              <Button disabled={!text.trim()} onClick={() => { action.mutate({ type: "comment", body: text }); discard(); }}>Comentar</Button>
              {d.pull && open && <Button variant="ghost" disabled={!text.trim()} onClick={() => { action.mutate({ type: "request_changes", body: text }); discard(); }}>Pedir cambios</Button>}
              {d.pull && open && canApprove(d.pull, d.viewer) && <Button variant="ghost" disabled={!text.trim()} onClick={() => { action.mutate({ type: "approve", body: text }); discard(); }}>Aprobar con mensaje</Button>}
              {text && <Button variant="ghost" className="ml-auto" onClick={discard}>Descartar</Button>}
            </div>
          </Card>
        </div>
        {!atBottom && (
          <span className="fixed bottom-5 right-5 z-40">
            <Tip text="Escribir un mensaje">
              <button type="button" aria-label="Escribir un mensaje" onClick={write} className="grid size-12 place-items-center rounded-full bg-blue-600 dark:bg-accent text-white shadow-lg ring-4 ring-white/60 transition-transform hover:scale-105 hover:bg-blue-700 dark:hover:bg-accent dark:ring-surface/60">
                <Icon i={icons.comment} className="size-5" />
              </button>
            </Tip>
          </span>
        )}
      </RefContext.Provider>}
    </div>
  );
}

type GithubAction = { type: string; title?: string; body?: string; method?: string; thread_id?: string; user_ids?: string[]; comment_id?: string; milestone_id?: string };

// State changes show at once and are applied to the cached detail before GitHub answers.
const logins = (d: GithubDetail, ids: string[] = []) => d.candidates.filter((u) => ids.includes(u.id));
// event appends what the viewer just wrote to the conversation, until GitHub returns the real one.
const event = (d: GithubDetail, type: string, body = "", state = ""): GithubDetail =>
  ({ ...d, timeline: [...d.timeline, { id: `pending-${Date.now()}`, type, actor: d.viewer, at: new Date().toISOString(), body, state, detail: "" }] });
const review = (d: GithubDetail, a: GithubAction, state: "APPROVED" | "CHANGES_REQUESTED"): GithubDetail => {
  if (!d.pull) return d;
  const base = a.body ? event(d, "PullRequestReview", a.body, state) : d;
  const approved = state === "APPROVED";
  return { ...base, pull: { ...d.pull,
    approved_by: approved ? [...new Set([...d.pull.approved_by, d.viewer])] : d.pull.approved_by.filter((l) => l !== d.viewer),
    changes_by: approved ? d.pull.changes_by.filter((l) => l !== d.viewer) : [...new Set([...d.pull.changes_by, d.viewer])],
    review_requests: d.pull.review_requests.filter((l) => l !== d.viewer) } };
};
const editEvent = (d: GithubDetail, a: GithubAction): GithubDetail => ({ ...d, timeline: d.timeline.map((e) => (e.id === a.comment_id ? { ...e, body: a.body ?? e.body } : e)) });
const editItem = (d: GithubDetail, a: GithubAction): GithubDetail => ({
  ...d, body: a.body ?? d.body,
  ...(d.pull && a.title ? { pull: { ...d.pull, title: a.title } } : {}),
  ...(d.issue && a.title ? { issue: { ...d.issue, title: a.title } } : {}),
});
const thread = (d: GithubDetail, id: string | undefined, f: (t: GithubDetail["threads"][number]) => GithubDetail["threads"][number]): GithubDetail =>
  ({ ...d, threads: d.threads.map((t) => (t.id === id ? f(t) : t)) });

// Every action shows at once and is applied to the cached detail before GitHub answers.
const optimistic: Record<string, (d: GithubDetail, a: GithubAction) => GithubDetail> = {
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
function useActionQueue(key: unknown[], onDone: (a: GithubAction) => void) {
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
        qc.invalidateQueries({ queryKey: ["github"] });
      } else qc.cancelQueries({ queryKey: key });
    });
    tail.current = run;
    return run;
  };
  return { mutateAsync, mutate: (a: GithubAction) => { mutateAsync(a).catch(() => {}); }, isPending: pending > 0, error };
}

// DetailSkeleton holds the layout of an issue or pull request while GitHub answers:
// header, actions, description and a few conversation messages.
function DetailSkeleton() {
  const bar = "rounded bg-slate-200 motion-safe:animate-pulse dark:bg-raised";
  return (
    <div role="status" aria-label="Cargando" className="space-y-3">
      <Card className="space-y-3">
        <div className="flex items-start gap-3">
          <div className={`h-6 w-16 rounded-full ${bar}`} />
          <div className="min-w-0 flex-1 space-y-2"><div className={`h-5 w-3/4 ${bar}`} /><div className={`h-3 w-1/3 ${bar}`} /></div>
        </div>
        <div className="flex flex-wrap gap-2">{[0, 1, 2, 3, 4].map((i) => <div key={i} className={`size-9 rounded-lg ${bar}`} />)}<div className={`ml-auto h-9 w-28 rounded-lg ${bar}`} /></div>
        <div className="flex items-center gap-2 border-t border-slate-100 pt-3 dark:border-border">{[0, 1, 2].map((i) => <div key={i} className={`size-6 rounded-full ${bar}`} />)}<div className={`h-3 w-24 ${bar}`} /></div>
        <div className="space-y-2 border-t border-slate-100 pt-3 dark:border-border">{[92, 85, 97, 60, 78].map((w, i) => <div key={i} className={`h-3 ${bar}`} style={{ width: `${w}%` }} />)}</div>
      </Card>
      {[0, 1, 2].map((k) => (
        <Card key={k} className="space-y-2">
          <div className="flex items-center gap-2"><div className={`size-7 rounded-full ${bar}`} /><div className={`h-3 w-28 ${bar}`} /><div className={`ml-auto h-3 w-16 ${bar}`} /></div>
          {[88, 70, k === 1 ? 45 : 80].map((w, i) => <div key={i} className={`h-3 ${bar}`} style={{ width: `${w}%`, animationDelay: `${k * 0.15}s` }} />)}
        </Card>
      ))}
    </div>
  );
}
