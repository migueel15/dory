import { useEffect, useRef, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Link, useParams } from "@tanstack/react-router";
import { api, type GithubDetail, type GithubPull, type GithubThread } from "../api/client";
import { Badge, Button, Card, Select, ErrorText, Md, Arrow, pill, toggleTask } from "../components/ui";
import { Avatar, PullStatus } from "./github";
import { Diff } from "./github-diff";
import { DefineIssueDialog } from "./define-issue";
import { GithubMark, Icon, IconButton, Menu, MenuItem, RefContext, RefMd, Tip, UserPicker, icons, useConfirm, MentionArea } from "../components/github-ui";

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
      <Tip text="Asignados"><span className="text-slate-400"><Icon d={icons.user} /></span></Tip>
      <span className="flex -space-x-1.5">
        {d.assignees.map((a) => (
          <Tip key={a.id} text={`${a.login} · clic para quitar`}>
            <button type="button" aria-label={`Quitar a ${a.login}`} disabled={busy} onClick={() => act("unassign", { user_ids: [a.id] })} className="group relative rounded-full">
              <img src={`https://github.com/${a.login}.png?size=48`} alt={a.login} className="size-6 rounded-full bg-slate-200 ring-2 ring-white dark:bg-hover dark:ring-surface" />
              <span className="absolute inset-0 grid place-items-center rounded-full bg-red-600/80 text-white opacity-0 transition-opacity group-hover:opacity-100"><Icon d={icons.x} className="size-3" /></span>
            </button>
          </Tip>
        ))}
      </span>
      <UserPicker tip="Asignar" title="Asignar a" users={assignable.filter((u) => u.id !== me?.id)} disabled={busy} onPick={(u) => act("assign", { user_ids: [u.id] })}
        extra={me && ((close) => (
          <MenuItem onClick={() => { close(); act("assign", { user_ids: [me.id] }); }}>
            <span className="grid size-6 place-items-center rounded-full bg-blue-100 text-blue-700 dark:bg-accent-subtle dark:text-accent-text"><Icon d={icons.user} className="size-3.5" /></span>
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
      <Tip text="Milestone"><span className="text-slate-400"><Icon d="M4 22V4M4 4h12l-2 4 2 4H4" /></span></Tip>
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
  const [confirm, dialog] = useConfirm();
  const merge = () => confirm({ title: "¿Mergear la PR?", text: `${method.name} de ${pull.repo}#${pull.number} (${pull.head_ref}) en ${pull.base_ref}.`, ok: "Mergear", run: () => act("merge", { method: method.value }) });
  return (<>{dialog}
    <Menu align="right" trigger={(open, toggle) => (
      <div className="inline-flex overflow-hidden rounded-lg bg-green-600 text-white shadow-sm">
        <button type="button" disabled={busy} onClick={merge} className="flex items-center gap-1.5 px-3 py-2 text-sm font-medium hover:bg-green-700 disabled:opacity-50">
          <Icon d={icons.merge} /> {method.name}
        </button>
        <button type="button" aria-label="Elegir método de merge" disabled={busy} onClick={toggle} className={`border-l border-green-500 px-2 hover:bg-green-700 ${open ? "bg-green-700" : ""}`}>
          <Icon d={icons.chevron} />
        </button>
      </div>
    )}>
      {(close) => methods.map((m) => (
        <MenuItem key={m.value} onClick={() => { setMethod(m); close(); }}>
          <span className="grid size-5 place-items-center text-green-600">{m.value === method.value && <Icon d={icons.check} />}</span>
          <span><span className="block font-medium">{m.name}</span><span className="block text-xs text-slate-500">{m.hint}</span></span>
        </MenuItem>
      ))}
    </Menu></>
  );
}

function PullActions({ pull, me, act, busy, diff, toggleDiff }: { pull: GithubPull; me: string; act: Act; busy: boolean; diff: boolean; toggleDiff: () => void }) {
  const [confirm, dialog] = useConfirm();
  const view = <IconButton icon={icons.code} tip={diff ? "Ocultar diff" : "Ver diff"} tone={diff ? "active" : "plain"} onClick={toggleDiff} />;
  if (pull.state !== "OPEN") return <>{dialog}{view}{pull.state === "CLOSED" && <IconButton icon={icons.reopen} tip="Reabrir" disabled={busy} onClick={() => confirm({ title: "¿Reabrir la PR?", text: "Volverá a estar abierta para revisión y merge.", ok: "Reabrir", run: () => act("reopen_pull") })} />}</>;
  return (
    <>
      {dialog}
      {view}
      {pull.draft
        ? <IconButton icon={icons.ready} tip="Marcar como lista para revisión" disabled={busy} onClick={() => confirm({ title: "¿Marcar como lista?", text: "Saldrá de draft y se avisará a los revisores.", ok: "Marcar como lista", run: () => act("ready") })} />
        : <IconButton icon={icons.draft} tip="Pasar a draft" disabled={busy} onClick={() => confirm({ title: "¿Pasar a draft?", text: "No se podrá mergear hasta que vuelva a estar lista.", ok: "Pasar a draft", run: () => act("draft") })} />}
      {canApprove(pull, me) && <IconButton icon={icons.check} tip="Aprobar" tone="green" disabled={busy} onClick={() => act("approve")} />}
      <IconButton icon={icons.x} tip="Cerrar sin mergear" tone="red" disabled={busy} onClick={() => confirm({ title: "¿Cerrar la PR sin mergear?", text: "Se cerrará sin integrar los cambios. Podrás reabrirla después.", ok: "Cerrar PR", danger: true, run: () => act("close_pull") })} />
      <span className="ml-auto"><MergeButton pull={pull} act={act} busy={busy} /></span>
    </>
  );
}

// IssueActions closes an issue as completed or not planned, or reopens it.
function IssueActions({ open, act, busy }: { open: boolean; act: Act; busy: boolean }) {
  const [confirm, dialog] = useConfirm();
  if (!open) return <>{dialog}<IconButton icon={icons.reopen} tip="Reabrir issue" disabled={busy} onClick={() => confirm({ title: "¿Reabrir la issue?", text: "Volverá a estar abierta.", ok: "Reabrir", run: () => act("reopen_issue") })} /></>;
  return (
    <>
      {dialog}
      <IconButton icon={icons.ready} tip="Cerrar como completada" tone="green" disabled={busy} onClick={() => confirm({ title: "¿Cerrar como completada?", text: "Se marcará como resuelta. Podrás reabrirla después.", ok: "Cerrar como completada", run: () => act("close_issue", { method: "COMPLETED" }) })} />
      <IconButton icon={icons.x} tip="Cerrar como no planificada" tone="red" disabled={busy} onClick={() => confirm({ title: "¿Cerrar como no planificada?", text: "Se cerrará sin resolver: no se va a hacer, está duplicada o ya no aplica.", ok: "Cerrar como no planificada", danger: true, run: () => act("close_issue", { method: "NOT_PLANNED" }) })} />
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
          {body && <Tip text="Responder citando"><button type="button" aria-label="Responder citando" onClick={() => onReply(quote(author, body))} className={small}><Icon d={icons.reply} /></button></Tip>}
          {mine && id && draft === undefined && <Tip text="Editar"><button type="button" aria-label="Editar" onClick={() => setDraft(body)} className={small}><Icon d={icons.edit} /></button></Tip>}
        </span>
      </div>
      {draft === undefined ? <RefMd onToggle={mine && id && !busy ? (line) => act({ type: editType, comment_id: id, body: toggleTask(body, line) }) : undefined}>{body}</RefMd> : (
        <div className="space-y-2">
          <textarea aria-label="Editar mensaje" autoFocus value={draft} onChange={(e) => setDraft(e.target.value)} onKeyDown={(e) => e.key === "Escape" && setDraft(undefined)} rows={Math.min(14, draft.split("\n").length + 2)} className="w-full rounded-md border border-slate-300 bg-white p-2 font-mono text-sm dark:border-border-strong dark:bg-surface" />
          <div className="flex justify-end gap-2">
            <Button variant="ghost" onClick={() => setDraft(undefined)}>Cancelar</Button>
            <Button disabled={busy || !draft.trim()} onClick={() => act({ type: editType, comment_id: id, body: draft }).then(() => setDraft(undefined))}>Guardar</Button>
          </div>
        </div>
      )}
    </div>
  );
}

function ThreadCard({ t, me, people, links, act, busy }: { t: GithubThread; me: string; people: string[]; links: GithubDetail["links"]; act: ThreadAct; busy: boolean }) {
  const [open, setOpen] = useState(!t.resolved);
  const [reply, setReply] = useState("");
  const send = () => act({ type: "reply_thread", thread_id: t.id, body: reply }).then(() => setReply(""));
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
        <MentionArea people={people} links={links} aria-label="Responder en el hilo" value={reply} onValue={setReply} rows={2} placeholder="Responder…" className="mt-2 w-full rounded border border-slate-300 bg-white p-2 text-sm dark:border-border dark:bg-surface" />
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
  const box = "w-full rounded-md border border-slate-300 bg-white px-2 py-1 dark:border-border-strong dark:bg-surface";
  return (
    <Card className="p-0">
      <div className="space-y-2 p-4">
        <div className="flex items-start gap-3">
          <span className={`mt-0.5 shrink-0 rounded-full px-2.5 py-0.5 text-xs font-semibold text-white ${stateColor[e.state] ?? "bg-slate-500"}`}>{stateName[e.state] ?? e.state}</span>
          {draft
            ? <input aria-label="Título" autoFocus value={draft.title} onChange={(ev) => setDraft({ ...draft, title: ev.target.value })} onKeyDown={(ev) => ev.key === "Escape" && setDraft(undefined)} className={`${box} min-w-0 flex-1 text-lg font-bold`} />
            : <h1 className="min-w-0 flex-1 break-words text-lg font-bold leading-snug">{e.title}</h1>}
          {(draft ? (
            <span className="flex shrink-0 gap-1">
              <Tip text="Guardar"><button type="button" aria-label="Guardar" disabled={busy || !draft.title.trim()} onClick={save} className="grid size-8 place-items-center rounded-lg bg-green-600 text-white hover:bg-green-700 disabled:opacity-50"><Icon d={icons.check} /></button></Tip>
              <Tip text="Cancelar"><button type="button" aria-label="Cancelar" onClick={() => setDraft(undefined)} className="grid size-8 place-items-center rounded-lg text-slate-500 hover:bg-slate-100 dark:hover:bg-raised"><Icon d={icons.x} /></button></Tip>
            </span>
          ) : (
            <Tip text="Editar título y descripción"><button type="button" aria-label="Editar título y descripción" onClick={() => setDraft({ title: e.title, body: d.body })} className="grid size-8 shrink-0 place-items-center rounded-lg text-slate-500 transition-colors hover:bg-slate-100 hover:text-slate-900 dark:text-muted dark:hover:bg-raised dark:hover:text-white"><Icon d={icons.edit} /></button></Tip>
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
            <span className="flex items-center gap-2"><Icon d={icons.user} />{d.assignees.map((a) => <Avatar key={a.id} login={a.login} />)}</span>
          )}
          <MilestonePicker d={d} act={act} busy={busy} />
          <span className="inline-flex items-center gap-1.5"><Icon d={icons.comment} />{d.issue.comments} comentarios</span>
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
        ? <div className="border-t border-slate-100 px-4 py-3 dark:border-border"><MentionArea aria-label="Descripción" value={draft.body} onValue={(body) => setDraft({ ...draft, body })} people={people} links={d.links} rows={12} className={`${box} font-mono text-sm`} /></div>
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
    const vv = window.visualViewport;
    const show = () => el.scrollIntoView({ block: "center" });
    if (vv) { vv.addEventListener("resize", show, { once: true }); setTimeout(() => vv.removeEventListener("resize", show), 1000); }
  });
  const write = () => { bottom.current?.scrollIntoView({ behavior: "smooth", block: "end" }); focusBox(); };
  const [showDiff, setShowDiff] = useState(false);
  const [defining, setDefining] = useState(false);
  const [confirm, dialog] = useConfirm();
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
  const action = useMutation({
    mutationFn: (a: { type: string; title?: string; body?: string; method?: string; thread_id?: string; user_ids?: string[]; comment_id?: string }) => api.githubAct(full, kind, n, a),
    onSuccess: (_, a) => {
      if (a.body && !a.thread_id && !a.type.startsWith("edit_")) discard();
      qc.invalidateQueries({ queryKey: key });
      qc.invalidateQueries({ queryKey: ["github"] });
    },
  });
  const d = detail.data;
  // People who can be mentioned: the repo's users and everyone in the conversation.
  const people = d ? [...new Set([...d.candidates.map((u) => u.login), (d.pull ?? d.issue)!.author, ...d.timeline.map((e) => e.actor), ...d.threads.flatMap((t) => t.comments.map((c) => c.author))])].filter((p) => /^[\w-]+$/.test(p) && p !== d.viewer).sort() : [];
  const act: Act = (type, extra = {}) => action.mutate({ type, ...extra });
  const open = (d?.pull ?? d?.issue)?.state === "OPEN";
  return (
    <div className="space-y-3">
      <Link to="/github" className={pill}><Arrow back /> GitHub</Link>
      <ErrorText error={detail.error ?? action.error} />
      {dialog}
      {d && <RefContext.Provider value={{ owner: full.split("/")[0], repo: full.split("/")[1], links: d.links }}>
        <Header d={d} act={act} busy={action.isPending} people={people} actions={<>
          {d.pull && <PullActions pull={d.pull} me={d.viewer} act={act} busy={action.isPending} diff={showDiff} toggleDiff={() => setShowDiff(!showDiff)} />}
          {d.issue && open && <Button variant="ghost" className="text-xs" onClick={() => setDefining(true)}>Definir tareas</Button>}
          {d.issue && <IssueActions open={open} act={act} busy={action.isPending} />}
        </>} />
        {defining && <DefineIssueDialog repo={full} number={n} onClose={() => setDefining(false)} />}
        {showDiff && <Diff repo={full} pr={n} />}
        <Threads threads={d.threads} me={d.viewer} people={people} links={d.links} act={action.mutateAsync} busy={action.isPending} />
        {d.timeline.map((e, i) => (
          <div key={i} className={e.body ? "" : "px-3 text-xs text-slate-500 dark:text-muted"}>
            {e.body ? (
              <Card>
                <Message author={e.actor} verb={reviewStates[e.state] ?? labels[e.type] ?? e.type} at={e.at} body={e.body} mine={e.actor === d.viewer} editType={e.type === "PullRequestReview" ? "edit_review" : "edit_comment"} id={e.id} act={action.mutateAsync} busy={action.isPending}
                  onReply={(q) => { setText(q + text); write(); }} />
              </Card>
            ) : <>{e.actor} {reviewStates[e.state] ?? labels[e.type] ?? e.type} {e.detail} · {new Date(e.at).toLocaleString()}</>}
          </div>
        ))}
        <div ref={bottom}>
          <Card className="space-y-2">
            <MentionArea boxRef={box} people={people} links={d.links} id="composer" aria-label="Mensaje" value={text} onValue={setText} onKeyDown={(e) => e.key === "Escape" && box.current?.blur()} rows={4} placeholder="Escribe un mensaje…" className="w-full rounded-md border border-slate-300 bg-white p-2 text-sm dark:border-border-strong dark:bg-surface" />
            <div className="flex flex-wrap gap-2">
              <Button disabled={action.isPending || !text.trim()} onClick={() => action.mutate({ type: "comment", body: text })}>Comentar</Button>
              {d.pull && open && <Button variant="ghost" disabled={action.isPending || !text.trim()} onClick={() => action.mutate({ type: "request_changes", body: text })}>Pedir cambios</Button>}
              {d.pull && open && canApprove(d.pull, d.viewer) && <Button variant="ghost" disabled={action.isPending || !text.trim()} onClick={() => action.mutate({ type: "approve", body: text })}>Aprobar con mensaje</Button>}
              {text && <Button variant="ghost" className="ml-auto" onClick={discard}>Descartar</Button>}
            </div>
          </Card>
        </div>
        {!atBottom && (
          <span className="fixed bottom-5 right-5 z-40">
            <Tip text="Escribir un mensaje">
              <button type="button" aria-label="Escribir un mensaje" onClick={write} className="grid size-12 place-items-center rounded-full bg-blue-600 dark:bg-accent text-white shadow-lg ring-4 ring-white/60 transition-transform hover:scale-105 hover:bg-blue-700 dark:hover:bg-accent dark:ring-surface/60">
                <Icon d={icons.comment} className="size-5" />
              </button>
            </Tip>
          </span>
        )}
      </RefContext.Provider>}
    </div>
  );
}
