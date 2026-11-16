import { useEffect, useLayoutEffect, useRef, useState, type RefObject } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Link, useNavigate, useParams } from "@tanstack/react-router";
import { ArrowUp, Brain, CaretRight, CheckCircle, Circle, CircleNotch, GitBranch, GitDiff, Kanban, PencilSimpleLine, PushPin, Stop, Trash, Warning, WarningCircle, Wrench, XCircle } from "@phosphor-icons/react";
import { api, type Message, type ThreadView as View } from "../api/client";
import { useThreadLive } from "../api/live";
import { Button, ErrorText, Md, Textarea, pill } from "../components/ui";
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
  const scroller = useRef<HTMLDivElement>(null);
  const stick = useRef(true);
  const t = view.data;
  // Follow the end of the conversation unless the user scrolled up.
  useLayoutEffect(() => {
    const el = scroller.current;
    if (el && stick.current) el.scrollTop = el.scrollHeight;
  }, [t?.messages]);
  if (!threadId) return null;
  if (view.error) return <div className="p-4"><ErrorText error={view.error} /></div>;
  return (
    <div className="flex min-h-0 flex-1 flex-col">
      <div ref={scroller} onScroll={(e) => { const el = e.currentTarget; stick.current = el.scrollHeight - el.scrollTop - el.clientHeight < 80; }}
        className="min-h-0 flex-1 overflow-y-auto">
        <div className="mx-auto max-w-3xl space-y-3 px-3 py-4">
          {t?.messages.map((m) => <Entry key={m.id} m={m} />)}
          {t?.status === "working" && !t.messages.some((m) => m.streaming) && <p className="flex items-center gap-2 text-sm text-slate-500 dark:text-muted"><CircleNotch className="size-4 animate-spin" />Trabajando…</p>}
          {t && !t.local && <p className="rounded-md bg-amber-50 px-3 py-2 text-sm text-amber-800 dark:bg-amber-950/40 dark:text-amber-300">Este hilo corre en otro nodo.</p>}
        </div>
      </div>
      {t && <Composer t={t} onSent={() => { stick.current = true; }} />}
    </div>
  );
}

export function Entry({ m }: { m: Message }) {
  switch (m.role) {
    case "user":
      return <div className="ml-auto w-fit max-w-[85%] whitespace-pre-wrap rounded-2xl rounded-br-sm bg-slate-200 px-3.5 py-2 text-sm dark:bg-raised">{m.text}</div>;
    case "assistant":
      return <div className="prose-sm text-sm"><Md>{m.text}</Md>{m.streaming && <span className="ml-0.5 inline-block h-4 w-1.5 animate-pulse bg-slate-400 align-middle" />}</div>;
    case "reasoning":
      return <Fold icon={<Brain className="size-4" />} title={m.streaming ? "Pensando…" : "Razonamiento"} body={m.text} muted />;
    case "tool":
      return <Fold icon={toolIcon(m.status)} title={m.title || "Herramienta"} detail={m.detail} body={m.text} mono />;
    case "plan":
      return (
        <div className="rounded-lg border border-slate-200 p-3 text-sm dark:border-border">
          <p className="mb-1 font-medium">Plan</p>
          {(m.detail || m.text).split("\n").map((l, i) => <p key={i} className={l.startsWith("[x]") ? "text-slate-400 line-through" : ""}>{l.replace(/^\[.\]\s*/, l.startsWith("[~]") ? "▸ " : "· ")}</p>)}
        </div>
      );
    case "interrupt":
      return <p className="text-center text-xs text-slate-500 dark:text-muted">— {m.text} —</p>;
    default:
      return (
        <div className={`rounded-md px-3 py-2 text-sm ${m.status === "failed" ? "bg-red-50 text-red-700 dark:bg-red-950/40 dark:text-red-300" : "bg-amber-50 text-amber-900 dark:bg-amber-950/40 dark:text-amber-200"}`}>
          {m.title && <p className="font-medium">{m.title}</p>}
          <Md>{m.text}</Md>
        </div>
      );
  }
}

function toolIcon(status?: string) {
  if (status === "running") return <CircleNotch className="size-4 animate-spin text-blue-500" />;
  if (status === "failed") return <Warning className="size-4 text-red-500" />;
  if (status === "completed") return <CheckCircle className="size-4 text-emerald-600" />;
  return <Wrench className="size-4" />;
}

function Fold({ icon, title, detail, body, muted, mono }: { icon: React.ReactNode; title: string; detail?: string; body?: string; muted?: boolean; mono?: boolean }) {
  const [open, setOpen] = useState(false);
  return (
    <div className={`text-sm ${muted ? "text-slate-500 dark:text-muted" : ""}`}>
      <button type="button" onClick={() => setOpen(!open)} className="flex w-full min-w-0 items-center gap-2 rounded px-1 py-0.5 text-left hover:bg-slate-100 dark:hover:bg-raised" disabled={!body}>
        {icon}
        <span className="shrink-0 font-medium">{title}</span>
        {detail && <span className="min-w-0 truncate font-mono text-xs text-slate-500 dark:text-muted">{detail.split("\n")[0]}</span>}
        {body && <CaretRight className={`ml-auto size-3.5 shrink-0 transition-transform ${open ? "rotate-90" : ""}`} />}
      </button>
      {open && body && (mono
        ? <pre className="mt-1 max-h-80 overflow-auto whitespace-pre-wrap rounded bg-slate-100 p-2 font-mono text-xs dark:bg-raised">{detail && detail.includes("\n") ? `${detail}\n\n` : ""}{body}</pre>
        : <div className="mt-1 border-l-2 border-slate-200 pl-3 dark:border-border"><Md>{body}</Md></div>)}
    </div>
  );
}

function Composer({ t, onSent }: { t: View; onSent: () => void }) {
  const qc = useQueryClient();
  const [text, setText] = useState("");
  const box = useRef<HTMLTextAreaElement>(null);
  useLayoutEffect(() => { const el = box.current; if (el) { el.style.height = "auto"; el.style.height = `${Math.min(el.scrollHeight, 320)}px`; } }, [text]);
  const send = useMutation({
    mutationFn: (body: string) => api.send(t.id, body),
    onSuccess: () => { setText(""); onSent(); qc.invalidateQueries({ queryKey: ["thread", t.id] }); },
  });
  const stop = useMutation({ mutationFn: () => api.interrupt(t.id) });
  const submit = () => { if (text.trim() && !send.isPending) send.mutate(text.trim()); };
  if (!t.local) return null;
  return (
    <div className="shrink-0 border-t border-slate-200 p-3 dark:border-border">
      <div className="mx-auto max-w-3xl">
        {t.queue.length > 0 && (
          <div className="mb-2 space-y-1">
            {t.queue.map((q) => <p key={q.run_id} className="truncate rounded bg-slate-100 px-2 py-1 text-xs text-slate-600 dark:bg-raised dark:text-muted">En cola: {q.text}</p>)}
          </div>
        )}
        <div className="flex items-end gap-2 rounded-xl border border-slate-300 bg-white p-2 focus-within:border-slate-500 dark:border-border dark:bg-surface">
          <Textarea bare ref={box} rows={1} value={text} onChange={(e) => setText(e.target.value)} placeholder={t.running ? "Escribe; se enviará al terminar el turno" : "Escribe un mensaje"} aria-label="Mensaje"
            onKeyDown={(e) => { if (e.key === "Enter" && !e.shiftKey && !e.nativeEvent.isComposing && window.matchMedia("(min-width: 640px)").matches) { e.preventDefault(); submit(); } }}
            className="max-h-80 min-h-9 flex-1 resize-none" />
          {t.running && <Button type="button" variant="ghost" title="Interrumpir" onClick={() => stop.mutate()} disabled={stop.isPending}><Stop className="size-4" weight="fill" /></Button>}
          <Button type="button" title="Enviar" onClick={submit} disabled={!text.trim() || send.isPending}><ArrowUp className="size-4" weight="bold" /></Button>
        </div>
        <ErrorText error={send.error ?? stop.error} />
      </div>
    </div>
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
        <div className="mx-auto max-w-3xl space-y-3 px-3 py-4">
          <ErrorText error={view.error ?? adopt.error} />
          <p className="rounded-md bg-slate-100 px-3 py-2 text-xs text-slate-600 dark:bg-raised dark:text-muted">Conversación iniciada en {provider === "claude" ? "Claude Code" : "Codex"}. Se muestra en solo lectura; al continuarla en Dory, el agente retoma la misma sesión en su carpeta.</p>
          {view.data?.messages.map((m) => <Entry key={m.id} m={m} />)}
        </div>
      </div>
    </div>
  );
}
