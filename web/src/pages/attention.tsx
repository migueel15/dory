import { ThreadLink } from "../components/thread-link";
import { useState, type ReactNode } from "react";
import { ArrowSquareOut, CheckCircle, Clock, FolderSimple, Flask, GitPullRequest, Prohibit, Robot, WarningCircle, type Icon as PhIcon } from "@phosphor-icons/react";
import { ItemTitle } from "../components/item-title";
import { ItemForm } from "./items";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Link } from "@tanstack/react-router";
import { api, type Item } from "../api/client";
import { useThreadLive } from "../api/live";
import { ThreadStatus } from "../components/thread-status";
import { PendingRequest } from "../components/pending-request";
import { Badge, Button, Card, ErrorText, Notice, PageHeader, StatusPill, Textarea } from "../components/ui";
import { useProject, useProjectThreads } from "../project";

export const useT3 = (enabled = true) => useQuery({ queryKey: ["t3-status"], queryFn: api.t3Status, refetchInterval: 60_000, enabled });
export const useBlocked = (enabled = true) => useQuery({ queryKey: ["items", "blocked"], queryFn: () => api.items.list({ status: "blocked" }), enabled });

// useAttention lists the threads waiting for the user and the blocked items,
// limited to the selected project (threads count when linked to its items).
export const useValidating = (enabled = true) => useQuery({ queryKey: ["items", "validating"], queryFn: () => api.items.list({ status: "validating" }), enabled });

function useAttention(enabled = true) {
  const project = useProject();
  const t3 = useT3(enabled);
  const blocked = useBlocked(enabled);
  const validating = useValidating(enabled);
  const linked = useProjectThreads(enabled ? project : "");
  const threads = t3.data?.flatMap((s) => s.threads.filter((t) => t.attention && (!linked.keys || linked.keys.has(`${s.instance_id}:${t.id}`))).map((t) => ({ ...t, status_: s }))) ?? [];
  const stuck = blocked.data?.filter((i) => !project || i.project_id === project) ?? [];
  const toValidate = validating.data?.filter((i) => !project || i.project_id === project) ?? [];
  return { t3, blocked, validating, threads, stuck, toValidate, error: t3.error ?? blocked.error ?? validating.error ?? linked.error };
}

export function useAttentionCount(enabled: boolean) {
  const { threads, stuck, toValidate } = useAttention(enabled);
  return threads.length + stuck.length + toValidate.length;
}

function ThreadReply({ id }: { id: string }) {
  const [open, setOpen] = useState(false);
  const [text, setText] = useState("");
  const qc = useQueryClient();
  const ctx = useQuery({ queryKey: ["t3-context", id], queryFn: () => api.t3Context(id), enabled: open, refetchInterval: 60_000 });
  useThreadLive(open ? id : undefined, ["t3-context", id]);
  const refresh = () => {
    qc.invalidateQueries({ queryKey: ["t3-context", id] });
    qc.invalidateQueries({ queryKey: ["t3-status"] });
  };
  const key = ["t3-context", id];
  // The own message shows at once; a failed send puts the previous chat back.
  const send = useMutation({
    mutationFn: (msg: string) => api.t3Send(id, msg),
    onMutate: async (msg) => {
      await qc.cancelQueries({ queryKey: key });
      const prev = qc.getQueryData<typeof ctx.data>(key);
      if (prev) qc.setQueryData(key, { ...prev, messages: [...prev.messages, { role: "user", text: msg } as (typeof prev.messages)[number]] });
      setText("");
      return { prev };
    },
    onError: (_e, msg, c) => { if (c?.prev) qc.setQueryData(key, c.prev); setText(msg); },
    onSettled: refresh,
  });
  if (!open) return <Button variant="ghost" onClick={() => setOpen(true)}>Responder</Button>;
  const pending = ctx.data?.pending;
  return (
    <div className="space-y-2">
      <ErrorText error={ctx.error ?? send.error} />
      <div className="space-y-1 text-sm">
        {ctx.data?.messages.map((m, i) => (
          <p key={i} className="break-words whitespace-pre-wrap"><span className="text-slate-500 dark:text-muted">{m.role}: </span>{m.text}</p>
        ))}
      </div>
      {pending && <PendingRequest key={pending.id} threadId={id} pending={pending} onDone={refresh} />}
      <Textarea value={text} onChange={(e) => setText(e.target.value)} placeholder="Mensaje para el agente" />
      <Button disabled={send.isPending || !text.trim()} onClick={() => send.mutate(text.trim())}>Enviar mensaje</Button>
    </div>
  );
}

const ROLE: Record<string, string> = { refine: "Definición", triage: "Análisis", implementation: "Implementación", review: "Revisión", replan: "Revisión del plan" };
const ago = (iso?: string | null) => {
  if (!iso) return "";
  const m = Math.max(0, Math.round((Date.now() - new Date(iso).getTime()) / 60_000));
  return m < 1 ? "ahora" : m < 60 ? `hace ${m} min` : m < 1440 ? `hace ${Math.round(m / 60)} h` : `hace ${Math.round(m / 1440)} d`;
};
const prOf = (refs: string[]) => refs.map((r) => r.match(/github\.com\/([^/]+)\/([^/]+)\/pull\/(\d+)/)).find(Boolean);

function Section({ icon: Icon, title, count, tone, children }: { icon: PhIcon; title: string; count: number; tone: string; children: ReactNode }) {
  if (count === 0) return null;
  return (
    <section className="space-y-2">
      <h2 className="flex items-center gap-2 text-sm font-semibold"><Icon weight="duotone" aria-hidden="true" className={`size-5 ${tone}`} />{title}<span className="rounded-md bg-slate-100 px-1.5 text-xs font-medium tabular-nums text-slate-600 dark:bg-raised dark:text-text-secondary">{count}</span></h2>
      <div className="space-y-2">{children}</div>
    </section>
  );
}

// ItemLine is the Dory item behind a card: title, project, status, last progress and PR.
function ItemLine({ item, projectName }: { item: Item; projectName?: string }) {
  const pr = prOf(item.refs);
  return (
    <div className="space-y-1 rounded-md bg-slate-50 p-2 text-sm dark:bg-raised/60">
      <div className="flex flex-wrap items-center gap-x-2 gap-y-1">
        <span className="min-w-0 truncate font-medium"><ItemTitle item={item} /></span>
        {projectName && <Badge>{projectName}</Badge>}
        {pr && <Link to="/github/$owner/$repo/$kind/$number" params={{ owner: pr[1], repo: pr[2], kind: "pulls", number: pr[3] }} onClick={(e) => e.stopPropagation()} className="inline-flex items-center gap-1 text-xs text-slate-500 hover:underline dark:text-muted"><GitPullRequest aria-hidden="true" className="size-3.5" />#{pr[3]}</Link>}
      </div>
      {item.progress && <p className="line-clamp-2 text-xs text-slate-500 dark:text-muted">{item.progress}</p>}
    </div>
  );
}

export function Attention() {
  const { t3, blocked, validating, threads, stuck, toValidate, error } = useAttention();
  const envs = useQuery({ queryKey: ["environments"], queryFn: () => api.environments.list() });
  const links = useQuery({ queryKey: ["item-threads"], queryFn: () => api.itemThreads.list() });
  const items = useQuery({ queryKey: ["items", "all"], queryFn: () => api.items.list() });
  const projects = useQuery({ queryKey: ["projects"], queryFn: () => api.projects.list() });
  const [open, setOpen] = useState<Item | null>(null);
  const down = t3.data?.filter((s) => s.state !== "available") ?? [];
  const envName = (id: string) => envs.data?.find((e) => e.id === id)?.name ?? "";
  const projectName = (id: string | null) => (id ? projects.data?.find((p) => p.id === id)?.name : undefined);
  const linkOf = (instance: string, thread: string) => links.data?.filter((l) => l.instance_id === instance && l.thread_id === thread).sort((a, b) => b.created_at.localeCompare(a.created_at))[0];
  const itemOf = (id?: string) => items.data?.find((i) => i.id === id);
  const meta = "text-xs text-slate-500 dark:text-muted";
  const total = threads.length + stuck.length + toValidate.length;
  return (
    <div className="space-y-5">
      <PageHeader title="Atención" subtitle={total ? `${total} pendientes` : undefined} />
      <ErrorText error={error} />
      {down.length > 0 && <p className="flex items-center gap-1.5 text-xs text-slate-500 dark:text-muted"><WarningCircle aria-hidden="true" className="size-4 text-amber-500" />Sin datos de {down.map((s) => envName(s.environment_id) || s.base_url).join(", ")}: no disponible.</p>}
      {(t3.isPending || blocked.isPending) && <div className="space-y-2">{[0, 1, 2].map((i) => <div key={i} className="h-24 rounded-lg bg-slate-100 motion-safe:animate-pulse dark:bg-raised" />)}</div>}

      <Section icon={Robot} title="Agentes esperándote" count={threads.length} tone="text-amber-500">
        {threads.map((t) => {
          const link = linkOf(t.status_.instance_id, t.id);
          const item = itemOf(link?.item_id);
          const failed = ["failed", "error", "interrupted"].includes(t.status.toLowerCase());
          return (
            <div key={t.id} onClick={() => item && setOpen(item)} className={item ? "cursor-pointer" : ""}><Card className={`space-y-3 ${item ? "transition-colors hover:bg-slate-50 dark:hover:bg-raised" : ""}`}>
              <div className="flex items-start gap-2">
                <div className="min-w-0 flex-1 space-y-1">
                  <div className="flex flex-wrap items-center gap-2">
                    <StatusPill tone={failed ? "red" : "amber"}>{failed ? "Ha fallado" : "Espera tu respuesta"}</StatusPill>
                    {link && <Badge>{ROLE[link.role] ?? link.role}</Badge>}
                  </div>
                  <div className="break-words font-medium">{t.title}</div>
                  <div className={meta}>{[envName(t.status_.environment_id), t.model, t.effort, ago(t.updated_at)].filter(Boolean).join(" · ")}</div>
                </div>
                <ThreadLink onClick={(e) => e.stopPropagation()} url={t.url} environmentId={t.t3_environment_id} threadId={t.id} className="inline-flex shrink-0 items-center gap-1 rounded-md border border-slate-200 px-2 py-1 text-xs hover:bg-slate-50 dark:border-border dark:hover:bg-raised">Abrir en T3 <ArrowSquareOut aria-hidden="true" className="size-3.5" /></ThreadLink>
                <Link onClick={(e) => e.stopPropagation()} to="/threads/$threadId" params={{ threadId: t.id }} className="inline-flex shrink-0 items-center gap-1 rounded-md border border-slate-200 px-2 py-1 text-xs hover:bg-slate-50 dark:border-border dark:hover:bg-raised">Chat</Link>
              </div>
              {item ? <ItemLine item={item} projectName={projectName(item.project_id)} /> : <p className={meta}>Hilo sin item de Dory asociado.</p>}
              <div onClick={(e) => e.stopPropagation()}><ThreadReply id={t.id} /></div>
            </Card></div>
          );
        })}
      </Section>

      <Section icon={Prohibit} title="Bloqueados" count={stuck.length} tone="text-red-500">
        {stuck.map((i) => (
          <div key={i.id} onClick={() => setOpen(i)} className="cursor-pointer"><Card className="flex gap-3 transition-colors hover:bg-slate-50 dark:hover:bg-raised">
            <span className="grid size-10 shrink-0 place-items-center rounded-lg bg-red-50 text-red-600 dark:bg-red-950/50 dark:text-red-400"><Prohibit weight="duotone" aria-hidden="true" className="size-5" /></span>
            <div className="min-w-0 flex-1 space-y-1.5">
              <div className="flex items-start gap-2">
                <span className="min-w-0 flex-1 font-medium"><ItemTitle item={i} /></span>
                <StatusPill tone="red">Bloqueado</StatusPill>
              </div>
              <p className="break-words text-sm text-slate-700 dark:text-text">{i.blocked_reason || <span className="italic text-slate-400 dark:text-muted">Sin motivo indicado</span>}</p>
              <div className={`flex flex-wrap items-center gap-x-3 gap-y-1 ${meta}`}>
                {projectName(i.project_id) && <span className="inline-flex items-center gap-1"><FolderSimple aria-hidden="true" className="size-3.5" />{projectName(i.project_id)}</span>}
                <span className="inline-flex items-center gap-1"><Clock aria-hidden="true" className="size-3.5" />Bloqueado {ago(i.updated_at)}</span>
                {i.progress && <span className="min-w-0 truncate">{i.progress}</span>}
              </div>
            </div>
          </Card></div>
        ))}
      </Section>

      <Section icon={Flask} title="Por validar" count={toValidate.length} tone="text-violet-500">
        {toValidate.map((i) => {
          const pr = prOf(i.refs);
          return (
            <div key={i.id} onClick={() => setOpen(i)} className="cursor-pointer"><Card className="space-y-2 transition-colors hover:bg-slate-50 dark:hover:bg-raised">
              <div className="flex items-start gap-2">
                <span className="min-w-0 flex-1 font-medium"><ItemTitle item={i} /></span>
                {projectName(i.project_id) && <Badge>{projectName(i.project_id)}</Badge>}
              </div>
              {i.progress && <p className="line-clamp-3 text-sm text-slate-600 dark:text-text-secondary">{i.progress}</p>}
              <div className="flex flex-wrap items-center gap-3 text-xs text-slate-500 dark:text-muted">
                <span>Terminado {ago(i.updated_at)}</span>
                {pr && <Link onClick={(e) => e.stopPropagation()} to="/github/$owner/$repo/$kind/$number" params={{ owner: pr[1], repo: pr[2], kind: "pulls", number: pr[3] }} className="inline-flex items-center gap-1 hover:underline"><GitPullRequest aria-hidden="true" className="size-3.5" />PR #{pr[3]}</Link>}
              </div>
            </Card></div>
          );
        })}
      </Section>

      {t3.isSuccess && blocked.isSuccess && validating.isSuccess && total === 0 && (
        <div className="flex flex-col items-center gap-2 py-16 text-sm text-slate-500 dark:text-muted"><CheckCircle weight="duotone" aria-hidden="true" className="size-8 text-emerald-500" />Nada pendiente. Todo en orden.</div>
      )}
      {open && <ItemForm item={open} onClose={() => setOpen(null)} />}
    </div>
  );
}
