import { Robot } from "@phosphor-icons/react";
import { useQueryClient } from "@tanstack/react-query";
import { Link,useParams } from "@tanstack/react-router";
import { useCallback,useEffect,useImperativeHandle,useRef,useState,type Ref } from "react";
import { type GithubDetail,type GithubPull } from "../../api/client";
import { useGithubWatch } from "../../api/live";
import { useCreatePullReview,useGithubDetail,usePullAgent } from "../../api/queries/github";
import { qk } from "../../api/queries/keys";
import { FileRefContext,type FileRef } from "../../components/refs";
import { Arrow,Badge,Button,Card,ErrorText,Input,Modal,keepAboveKeyboard,pill,toggleTask } from "../../components/ui";
import { DefineIssueDialog } from "../../pages/define-issue";
import { useLaunchForm } from "../composer/launch-picker";
import { useDictationInProgress } from "../composer/local-dictation";
import { ResizeHandle,ThreadChat } from "../thread/thread";
import { Assignees,IssueActions,MilestonePicker,PullActions,ReviewPicker } from "./detail-actions";
import { useActionQueue } from "./detail-optimistic";
import { Act,canApprove,labels,reviewStates } from "./detail-shared";
import { Message,Threads } from "./detail-threads";
import { StackBar } from "./github";
import { Diff,FullscreenHost } from "./github-diff";
import { GithubMark,Icon,MentionArea,RefContext,RefMd,Tip,icons } from "./github-ui";
import { Avatar,PullStatus } from "./pull-row";

// Header is the single card of the pull request or issue: what it is on top,
// its review state and people in the middle, and the actions at the bottom.
function Header({ d, actions, act, busy, people }: { d: GithubDetail; actions: React.ReactNode; act: Act; busy: boolean; people: string[] }) {
  const e = d.pull ?? d.issue!;
  const live = d.pull?.state === "OPEN";
  const refs = d.pull?.issues ?? d.issue?.pulls ?? [];
  const [owner, repo] = e.repo.split("/");
  const [draft, setDraft] = useState<{ title: string; body: string }>();
  const dictating = useDictationInProgress();
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
              <Tip text="Guardar"><button type="button" aria-label="Guardar" disabled={dictating || busy || !draft.title.trim()} onClick={save} className="grid size-8 place-items-center rounded-lg bg-green-600 text-white hover:bg-green-700 disabled:opacity-50"><Icon i={icons.check} /></button></Tip>
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
        ? <div className="border-t border-slate-100 px-4 py-3 dark:border-border"><MentionArea aria-label="Descripción" value={draft.body} onValue={(body) => setDraft((current) => current && { ...current, body })} people={people} links={d.links} rows={12} className={`${box} font-mono`} /></div>
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

type ComposerHandle = { quote: (q: string) => void };

// Composer owns the message text, so typing re-renders only the box and not the page or the diff.
function Composer({ ref, boxRef, people, links, pull, viewer, open, onSend }: { ref: Ref<ComposerHandle>; boxRef: React.RefObject<HTMLTextAreaElement | null>; people: string[]; links: GithubDetail["links"]; pull?: GithubPull; viewer: string; open: boolean; onSend: (type: string, body: string) => void }) {
  const [text, setText] = useState("");
  const dictating = useDictationInProgress();
  useImperativeHandle(ref, () => ({ quote: (q) => { if (!dictating) setText((t) => q + t); } }), [dictating]);
  const discard = () => { setText(""); boxRef.current?.blur(); };
  const send = (type: string) => { onSend(type, text); discard(); };
  return (
    <Card className="space-y-2">
      <MentionArea boxRef={boxRef} people={people} links={links} id="composer" aria-label="Mensaje" value={text} onValue={setText} onKeyDown={(e) => e.key === "Escape" && boxRef.current?.blur()} rows={4} placeholder="Escribe un mensaje…" className="p-2" />
      <div className="flex flex-wrap gap-2">
        <Button disabled={dictating || !text.trim()} onClick={() => send("comment")}>Comentar</Button>
        {pull && open && <Button variant="ghost" disabled={dictating || !text.trim()} onClick={() => send("request_changes")}>Pedir cambios</Button>}
        {pull && open && canApprove(pull, viewer) && <Button variant="ghost" disabled={dictating || !text.trim()} onClick={() => send("approve")}>Aprobar con mensaje</Button>}
        {text && <Button variant="ghost" className="ml-auto" disabled={dictating} onClick={discard}>Descartar</Button>}
      </div>
    </Card>
  );
}

const stateColor: Record<string, string> = { OPEN: "bg-green-600", CLOSED: "bg-red-600", MERGED: "bg-purple-600" };
const stateName: Record<string, string> = { OPEN: "Abierta", CLOSED: "Cerrada", MERGED: "Mergeada" };

export function GithubView() {
  const { owner, repo, kind, number } = useParams({ strict: false }) as Record<string, string>;
  const full = `${owner}/${repo}`;
  const n = Number(number);
  const qc = useQueryClient();
  const box = useRef<HTMLTextAreaElement>(null);
  const composer = useRef<ComposerHandle>(null);
  // On phones the keyboard shrinks the visual viewport after focus; keep the box above it.
  const focusBox = () => requestAnimationFrame(() => {
    const el = box.current; if (!el) return;
    el.focus({ preventScroll: true });
    keepAboveKeyboard(bottom.current);
  });
  const write = () => { bottom.current?.scrollIntoView({ behavior: "smooth", block: "end" }); focusBox(); };
  const [showDiff, setShowDiff] = useState(false);
  const onAsked = useCallback(() => setChat(true), []);
  const [defining, setDefining] = useState(false);
  useGithubWatch();
  const key = qk.githubDetail(full, kind, n);
  const detail = useGithubDetail(full, kind, n, { refetchInterval: 60_000 });
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
  // A pull request's review thread shows as a chat on the right, 60/40 by
  // default and resizable; asking from the diff always opens it.
  const agent = usePullAgent(full, n, { enabled: kind === "pulls" });
  const [launchReview, setLaunchReview] = useState(false);
  const createReview = useCreatePullReview(full, n, {
    onSuccess: () => { setLaunchReview(false); setChat(true); qc.invalidateQueries({ queryKey: qk.pullAgent(full, n) }); },
  });
  const [chat, setChat] = useState(true);
  const side = useRef<HTMLElement>(null);
  const [width, setWidth] = useState<number | null>(null);
  // Until dragged, the chat takes 40 % of the page.
  const frame = useRef<HTMLDivElement>(null);
  const [frameW, setFrameW] = useState(0);
  useEffect(() => {
    const el = frame.current;
    if (!el) return;
    const ro = new ResizeObserver(([e]) => setFrameW(e.contentRect.width));
    ro.observe(el);
    return () => ro.disconnect();
  }, [agent.data?.thread_id]);
  const panelW = width ?? Math.round((frameW || 1200) * 0.4);
  const reviewThread = agent.data?.available ? agent.data.thread_id : undefined;
  const showChat = !!reviewThread && chat;
  // File references in the chat open the diff on that line.
  const [focus, setFocus] = useState<FileRef & { seq: number }>();
  // With the chat open, the diff's full screen covers only the PR column.
  const [column, setColumn] = useState<HTMLElement | null>(null);
  const openRef = (ref: FileRef) => { setShowDiff(true); setFocus((p) => ({ ...ref, seq: (p?.seq ?? 0) + 1 })); };
  const page = (
    <div className="space-y-3">
      <div className="flex items-center justify-between gap-2">
        <Link to="/github" className={pill}><Arrow back /> GitHub</Link>
      </div>
      <ErrorText error={detail.error ?? action.error} />
      {detail.isPending && <DetailSkeleton />}
      {d && <RefContext.Provider value={{ owner: full.split("/")[0], repo: full.split("/")[1], links: d.links }}>
        {d.pull?.state === "OPEN" && <StackBar repo={full} number={n} />}
        <Header d={d} act={act} busy={false} people={people} actions={<>
          {d.pull && agent.data?.available === false && <Button disabled={createReview.isPending} onClick={() => { createReview.reset(); setLaunchReview(true); }}>Crear hilo de revisión</Button>}
          {launchReview && <ReviewLaunch busy={createReview.isPending} error={createReview.error} onClose={() => setLaunchReview(false)} onLaunch={(launch) => createReview.mutate(launch)} n={n} />}
          {d.pull && <PullActions pull={d.pull} me={d.viewer} act={act} busy={false} diff={showDiff} toggleDiff={() => setShowDiff(!showDiff)} />}
          {d.pull?.mergeable === "CONFLICTING" && open && <Button variant="ghost" className="text-xs" onClick={() => setDefining(true)}>Resolver conflictos</Button>}
          {d.issue && open && <Button variant="ghost" className="text-xs" onClick={() => setDefining(true)}>Definir tareas</Button>}
          {d.issue && <IssueActions open={open} act={act} busy={false} />}
        </>} />
        {defining && <DefineIssueDialog resolve={!!d.pull} repo={full} number={n} onClose={() => setDefining(false)} />}
        {showDiff && <Diff repo={full} pr={n} focus={focus} onAsked={onAsked} />}
        <Threads threads={d.threads} me={d.viewer} people={people} links={d.links} act={action.mutateAsync} busy={false} />
        {d.timeline.map((e, i) => (
          <div key={i} className={e.body ? "" : "px-3 text-xs text-slate-500 dark:text-muted"}>
            {e.body ? (
              <Card>
                <Message author={e.actor} verb={reviewStates[e.state] ?? labels[e.type] ?? e.type} at={e.at} body={e.body} mine={e.actor === d.viewer} editType={e.type === "PullRequestReview" ? "edit_review" : "edit_comment"} id={e.id} act={action.mutateAsync} busy={false}
                  onReply={(q) => { composer.current?.quote(q); write(); }} />
              </Card>
            ) : <>{e.actor} {reviewStates[e.state] ?? labels[e.type] ?? e.type} {e.detail} · {new Date(e.at).toLocaleString()}</>}
          </div>
        ))}
        <div ref={bottom}>
          <Composer ref={composer} boxRef={box} people={people} links={d.links} pull={d.pull ?? undefined} viewer={d.viewer} open={open} onSend={(type, body) => action.mutate({ type, body })} />
        </div>
        {!atBottom && !showChat && (
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
  // The page uses the full width only with the chat; alone it keeps its usual width.
  if (!reviewThread) return <div className="mx-auto max-w-6xl">{page}</div>;
  // The chat panel stays mounted and slides its width like the thread's changes
  // panel; its content is pinned right so it enters from the right edge.
  return (
    <div ref={frame} className="flex h-[calc(100dvh_-_4rem_-_1px_-_var(--tabbar))] min-h-0 sm:h-[calc(100dvh-5.5rem-2px)]">
      <div ref={setColumn} className="relative min-h-0 min-w-0 flex-1">
        {!chat && (
          // Pinned to the top right while the PR scrolls, so the agent opens from anywhere.
          <button type="button" onClick={() => setChat(true)} title="Abrir el agente de revisión" aria-label="Abrir el agente de revisión"
            className={`${pill} !px-2 absolute right-3 top-0 z-30 hidden bg-white shadow-sm lg:inline-flex dark:bg-surface`}><Robot className="size-4" /></button>
        )}
        <div className="h-full overflow-y-auto pr-2"><FullscreenHost.Provider value={column}><div
          // Same as a thread's chat: centred at its usual width inside the column, so the
          // column shrinking under the panel slides it left and narrows it only when it no longer fits.
          className="mx-auto max-w-6xl">{page}</div></FullscreenHost.Provider></div>
      </div>
      <aside ref={side} aria-label="Hilo de revisión" aria-hidden={!chat} style={{ "--panel-w": `${panelW}px` } as React.CSSProperties}
        className={`hidden min-h-0 overflow-hidden lg:flex lg:shrink-0 lg:transition-[width] lg:duration-300 lg:ease-out ${chat ? "lg:w-[var(--panel-w)]" : "lg:w-0"}`}>
        <div className="ml-auto flex min-h-0 w-[var(--panel-w)] shrink-0">
          <ResizeHandle panel={side} width={panelW} onResize={setWidth} storageKey={null} min={360} rest={420} />
          <div className="flex min-h-0 min-w-0 flex-1 flex-col">
            {/* Open, the toggle lives at the chat's top left; closing it puts it back in the page header. */}
            <div className="flex shrink-0 px-2 pt-1">
              <button type="button" onClick={() => setChat(false)} title="Cerrar el agente de revisión" aria-label="Cerrar el agente de revisión"
                className={`${pill} !px-2 !border-slate-500 !text-slate-900 dark:!text-white`}><Robot className="size-4" /></button>
            </div>
            <FileRefContext.Provider value={openRef}><ThreadChat threadId={reviewThread} /></FileRefContext.Provider>
          </div>
        </div>
      </aside>
    </div>
  );
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

// ReviewLaunch picks the agent of the pull request's side chat.
function ReviewLaunch({ n, busy, error, onClose, onLaunch }: { n: number; busy: boolean; error: unknown; onClose: () => void; onLaunch: (l: ReturnType<typeof useLaunchForm>["value"]) => void }) {
  const form = useLaunchForm({ section: "pull-review" });
  return (
    <Modal title={`Crear el hilo de revisión de #${n}`} onClose={onClose}>
      {form.fields}
      <p className="text-xs text-slate-500 dark:text-muted">Se crea un hilo para tus consultas sobre esta PR, en la copia local del repositorio. El agente no modifica nada salvo que se lo pidas.</p>
      <ErrorText error={form.error ?? error} />
      <Button disabled={!form.ready || busy} onClick={() => { form.remember(); onLaunch(form.value); }}>{busy ? "Creando…" : "Crear hilo"}</Button>
    </Modal>
  );
}
