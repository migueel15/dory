import { useEffect, useLayoutEffect, useRef, useState, type RefObject } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Link, useNavigate, useParams } from "@tanstack/react-router";
import { ArrowUp, Circle, CircleNotch, GitBranch, GitDiff, Kanban, ListBullets, PencilSimpleLine, PushPin, Stop, Trash, WarningCircle, XCircle } from "@phosphor-icons/react";
import { api, type ThreadView as View } from "../api/client";
import { useThreadLive } from "../api/live";
import { Button, ErrorText, pill } from "../components/ui";
import { Messages, ToEnd, useFollow } from "./chat";
import { ProviderIcon } from "../components/launch-picker";
import { Diff } from "./github-diff";

// ThreadView is a whole thread page: header, chat and, on demand, its changes.
export function ThreadView() {
  const { threadId } = useParams({ strict: false }) as { threadId: string };
  const [diff, setDiff] = useState(false);
  return (
    <div className="flex h-[calc(100dvh-4rem-1px)] min-h-0 sm:h-[calc(100dvh-3rem-2px)]">
      <div className="flex min-h-0 min-w-0 flex-1 flex-col">
        <Header threadId={threadId} diff={diff} toggleDiff={() => setDiff(!diff)} />
        <ThreadChat threadId={threadId} />
      </div>
      {diff && <Changes threadId={threadId} />}
    </div>
  );
}

function useThread(threadId?: string) {
  useThreadLive(threadId);
  return useQuery({ queryKey: ["thread", threadId], queryFn: () => api.thread(threadId!), enabled: !!threadId, staleTime: 5_000 });
}

function Header({ threadId, diff, toggleDiff }: { threadId: string; diff: boolean; toggleDiff: () => void }) {
  const qc = useQueryClient();
  const navigate = useNavigate();
  const t = useThread(threadId).data;
  const [editing, setEditing] = useState(false);
  const [title, setTitle] = useState("");
  const act = useMutation({
    mutationFn: (body: Parameters<typeof api.threadAction>[1]) => api.threadAction(threadId, body),
    onSuccess: (_, body) => {
      qc.invalidateQueries({ queryKey: ["thread", threadId] });
      qc.invalidateQueries({ queryKey: ["threads"] });
      if (body.action === "archive") navigate({ to: "/threads" });
    },
  });
  // Opening a thread marks it as read.
  useEffect(() => { if (t && !t.read_at && t.status !== "working") act.mutate({ action: "read" }); }, [t?.id, t?.status]); // eslint-disable-line react-hooks/exhaustive-deps
  if (!t) return <div className="h-12 shrink-0" />;
  return (
    <div className="flex shrink-0 flex-wrap items-center gap-2 border-b border-slate-200 px-3 py-2 dark:border-border">
      <ProviderIcon driver={t.provider} model={t.model} />
      {editing ? (
        <form className="min-w-0 flex-1" onSubmit={(e) => { e.preventDefault(); setEditing(false); if (title.trim()) act.mutate({ action: "rename", title: title.trim() }); }}>
          <input autoFocus value={title} onChange={(e) => setTitle(e.target.value)} onBlur={() => setEditing(false)} className="w-full rounded border border-slate-300 bg-transparent px-2 py-1 text-sm dark:border-border-strong" />
        </form>
      ) : (
        <h1 className="min-w-0 flex-1 truncate font-semibold" title={t.title}>{t.title || "Hilo"}</h1>
      )}
      <StatusDot status={t.status} />
      {t.branch && <span className="hidden items-center gap-1 font-mono text-xs text-slate-500 sm:flex dark:text-muted"><GitBranch className="size-3.5" />{t.branch}</span>}
      {t.task_id && <Link to="/tasks" search={{ task: t.task_id } as never} className={`${pill} !px-2`} title="Ver la tarea"><Kanban className="size-4" /></Link>}
      <button type="button" className={`${pill} !px-2`} title="Renombrar" onClick={() => { setTitle(t.title); setEditing(true); }}><PencilSimpleLine className="size-4" /></button>
      <button type="button" className={`${pill} !px-2 ${t.pinned_at ? "!text-amber-600" : ""}`} title={t.pinned_at ? "Desfijar" : "Fijar"} onClick={() => act.mutate({ action: t.pinned_at ? "unpin" : "pin" })}><PushPin className="size-4" weight={t.pinned_at ? "fill" : "duotone"} /></button>
      {t.local && <button type="button" className={`${pill} !px-2 ${diff ? "!border-slate-500" : ""}`} title="Cambios del hilo" onClick={toggleDiff}><GitDiff className="size-4" /></button>}
      <button type="button" className={`${pill} !px-2`} title="Archivar (borra su worktree)" onClick={() => confirm("¿Archivar el hilo? Se borra su worktree.") && act.mutate({ action: "archive" })}><Trash className="size-4" /></button>
      <ErrorText error={act.error} />
    </div>
  );
}

export function StatusDot({ status }: { status: string }) {
  if (status === "working") return <CircleNotch className="size-4 shrink-0 animate-spin text-blue-500" aria-label="Trabajando" />;
  if (status === "waiting") return <WarningCircle className="size-4 shrink-0 text-orange-500" weight="fill" aria-label="Te necesita" />;
  if (status === "error") return <XCircle className="size-4 shrink-0 text-red-500" weight="fill" aria-label="Error" />;
  return <Circle className="size-3 shrink-0 text-slate-400" weight="fill" aria-label="Inactivo" />;
}

// ThreadChat is the conversation and its composer; the PR page embeds it too.
export function ThreadChat({ threadId }: { threadId?: string }) {
  const view = useThread(threadId);
  const t = view.data;
  const { scroller, onScroll, follow, away } = useFollow([t?.messages, t?.queue]);
  if (!threadId) return null;
  if (view.error) return <div className="p-4"><ErrorText error={view.error} /></div>;
  return (
    <section className="relative flex min-h-0 flex-1 flex-col">
      <div ref={scroller} onScroll={onScroll} className="min-h-0 flex-1 overflow-y-auto py-3">
        <div className="mx-auto w-full max-w-3xl space-y-4 px-3 sm:px-6">
          {t && t.messages.length === 0 && <p className="py-10 text-center text-sm text-slate-500 dark:text-muted">Sin mensajes todavía. Escribe abajo para empezar.</p>}
          {t && <Messages key={t.id} messages={t.messages} running={t.running} />}
          {t && t.queue.length > 0 && (
            <section aria-label="Mensajes en cola" className="space-y-3 border-t border-dashed border-slate-300 pt-3 dark:border-border">
              {t.queue.map((q) => (
                <div key={q.run_id} className="msg-in flex flex-col items-end gap-1">
                  <div className="w-fit max-w-[85%] rounded-2xl rounded-br-md border border-dashed border-slate-400 px-4 py-2.5 text-slate-700 dark:border-border-strong dark:text-text-secondary">
                    <p className="whitespace-pre-wrap break-words text-sm">{q.text}</p>
                  </div>
                </div>
              ))}
            </section>
          )}
          {t && !t.local && <p className="rounded-md bg-amber-50 px-3 py-2 text-sm text-amber-800 dark:bg-amber-950/40 dark:text-amber-300">Este hilo corre en otro nodo.</p>}
        </div>
      </div>
      <ToEnd away={away} follow={follow} />
      {t && <Composer t={t} onSent={follow} />}
    </section>
  );
}

// Composer keeps the v1 look: a rounded box with the text on top and the
// send or stop button at its bottom right.
function Composer({ t, onSent }: { t: View; onSent: () => void }) {
  const qc = useQueryClient();
  const [text, setText] = useState("");
  const send = useMutation({
    mutationFn: (body: string) => api.send(t.id, body),
    onSuccess: () => { setText(""); onSent(); qc.invalidateQueries({ queryKey: ["thread", t.id] }); },
  });
  const stop = useMutation({ mutationFn: () => api.interrupt(t.id) });
  const submit = () => { if (text.trim() && !send.isPending) send.mutate(text.trim()); };
  if (!t.local) return null;
  const busy = t.running;
  return (
    <div className="mx-auto w-full max-w-3xl px-3 pb-4 pt-2 sm:px-6">
      <form onSubmit={(e) => { e.preventDefault(); submit(); }} className="relative z-10 rounded-3xl border border-slate-200 bg-white shadow-md dark:border-border-strong dark:bg-surface">
        <TextareaAutosize value={text} onChange={setText} onEnter={submit} />
        <div className="flex items-center gap-0.5 px-2 pb-2">
          <div className="flex min-w-0 flex-1 items-center gap-1.5 px-2 text-xs text-slate-500 dark:text-text-secondary">
            <ProviderIcon driver={t.provider} model={t.model} /><span className="truncate">{t.model || t.provider}{t.effort && <span className="text-slate-400"> · {t.effort}</span>}</span>
          </div>
          {busy && !text.trim() ? (
            <button type="button" onClick={() => stop.mutate()} disabled={stop.isPending} title="Parar el turno en curso" aria-label="Parar"
              className="grid size-9 shrink-0 place-items-center rounded-full bg-slate-900 text-white transition hover:bg-slate-700 disabled:opacity-50 dark:bg-white dark:text-slate-900 dark:hover:bg-slate-200">
              <Stop weight="fill" className="size-3.5" />
            </button>
          ) : (
            <button type="submit" disabled={!text.trim() || send.isPending} title={busy ? "Encolar (Enter)" : "Enviar (Enter)"} aria-label="Enviar"
              className="grid size-9 shrink-0 place-items-center rounded-full bg-slate-900 p-0 text-white transition hover:bg-slate-700 disabled:bg-slate-200 disabled:text-slate-400 dark:bg-white dark:text-slate-900 dark:hover:bg-slate-200 dark:disabled:bg-raised dark:disabled:text-muted">
              {send.isPending ? <span className="size-4 animate-spin rounded-full border-2 border-current border-t-transparent" /> : busy ? <ListBullets weight="bold" className="block size-4" /> : <ArrowUp weight="bold" className="block size-4" />}
            </button>
          )}
        </div>
        {(send.error || stop.error) && <div className="px-4 pb-2"><ErrorText error={send.error ?? stop.error} /></div>}
      </form>
    </div>
  );
}

function TextareaAutosize({ value, onChange, onEnter }: { value: string; onChange: (v: string) => void; onEnter: () => void }) {
  const area = useRef<HTMLTextAreaElement>(null);
  useLayoutEffect(() => { const el = area.current; if (el) { el.style.height = "auto"; el.style.height = `${Math.min(el.scrollHeight, 240)}px`; } }, [value]);
  return (
    <textarea ref={area} rows={1} value={value} onChange={(e) => onChange(e.target.value)} aria-label="Mensaje" placeholder="Escribe al hilo…"
      onKeyDown={(e) => { if (e.nativeEvent.isComposing) return; if (e.key === "Enter" && !e.shiftKey && window.matchMedia("(min-width: 640px)").matches) { e.preventDefault(); onEnter(); } }}
      className="block max-h-60 w-full resize-none break-words bg-transparent px-4 pb-1 pt-3.5 text-[15px] leading-6 !outline-none placeholder:text-slate-400" />
  );
}

function Changes({ threadId }: { threadId: string }) {
  const diff = useQuery({ queryKey: ["thread-diff", threadId], queryFn: () => api.threadDiff(threadId), refetchInterval: 5000 });
  return (
    <aside className="hidden w-[45%] min-w-[360px] shrink-0 flex-col border-l border-slate-200 lg:flex dark:border-border" aria-label="Cambios">
      <div className="flex shrink-0 items-center gap-2 border-b border-slate-200 px-3 py-2 text-sm font-medium dark:border-border"><GitDiff className="size-4" />Cambios{diff.data?.branch && <span className="font-mono text-xs text-slate-500">{diff.data.branch}</span>}</div>
      <div className="min-h-0 flex-1 overflow-auto">
        <ErrorText error={diff.error} />
        {diff.data && (diff.data.diff ? <Diff text={diff.data.diff} fill /> : <p className="p-4 text-sm text-slate-500 dark:text-muted">Sin cambios todavía.</p>)}
      </div>
    </aside>
  );
}

// ResizeHandle lets a side panel be dragged wider or narrower.
export function ResizeHandle({ panel, width, onResize, min = 320, rest = 420 }: { panel: RefObject<HTMLElement | null>; width: number; onResize: (w: number) => void; storageKey?: string | null; min?: number; rest?: number }) {
  return (
    <div role="separator" aria-orientation="vertical" tabIndex={0}
      className="w-1.5 shrink-0 cursor-col-resize bg-transparent hover:bg-slate-300 dark:hover:bg-border"
      onPointerDown={(e) => {
        const start = e.clientX, from = width;
        const right = panel.current?.getBoundingClientRect().right ?? window.innerWidth;
        const move = (ev: PointerEvent) => onResize(Math.max(min, Math.min(from + start - ev.clientX, right - rest)));
        const up = () => { window.removeEventListener("pointermove", move); window.removeEventListener("pointerup", up); };
        window.addEventListener("pointermove", move);
        window.addEventListener("pointerup", up);
      }} />
  );
}

// ExternalView shows a conversation started in a harness, read-only, and
// continues it as a Dory thread that resumes the same session.
export function ExternalView() {
  const { node, provider, id } = useParams({ strict: false }) as { node: string; provider: string; id: string };
  const navigate = useNavigate();
  const qc = useQueryClient();
  const view = useQuery({ queryKey: ["external", node, provider, id], queryFn: () => api.externalSession(node, provider, id) });
  const adopt = useMutation({
    mutationFn: () => api.adoptSession(node, provider, id),
    onSuccess: (r) => { qc.invalidateQueries({ queryKey: ["threads"] }); qc.invalidateQueries({ queryKey: ["external"] }); navigate({ to: "/threads/$threadId", params: { threadId: r.thread_id } }); },
  });
  const s = view.data?.session;
  return (
    <div className="flex min-h-0 flex-1 flex-col">
      <div className="flex shrink-0 flex-wrap items-center gap-2 border-b border-slate-200 px-3 py-2 dark:border-border">
        <ProviderIcon driver={provider} />
        <h1 className="min-w-0 flex-1 truncate font-semibold">{s?.title ?? "Conversación"}</h1>
        {s?.dir && <span className="hidden truncate font-mono text-xs text-slate-500 sm:block">{s.dir}</span>}
        <Button onClick={() => adopt.mutate()} disabled={adopt.isPending}>Continuar en Dory</Button>
      </div>
      <div className="min-h-0 flex-1 overflow-y-auto">
        <div className="mx-auto w-full max-w-3xl space-y-4 px-3 py-3 sm:px-6">
          <ErrorText error={view.error ?? adopt.error} />
          <p className="rounded-md bg-slate-100 px-3 py-2 text-xs text-slate-600 dark:bg-raised dark:text-muted">Conversación iniciada en {provider === "claude" ? "Claude Code" : "Codex"}. Se muestra en solo lectura; al continuarla en Dory, el agente retoma la misma sesión en su carpeta.</p>
          {view.data && <Messages messages={view.data.messages} running={false} />}
        </div>
      </div>
    </div>
  );
}
