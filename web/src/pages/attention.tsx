import { ThreadLink } from "../components/thread-link";
import { useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Link } from "@tanstack/react-router";
import { api } from "../api/client";
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

export function Attention() {
  const { t3, blocked, validating, threads, stuck, toValidate, error } = useAttention();
  const envs = useQuery({ queryKey: ["environments"], queryFn: () => api.environments.list() });
  const down = t3.data?.filter((s) => s.state !== "available") ?? [];
  const envName = (id: string) => envs.data?.find((e) => e.id === id)?.name ?? "";
  return (
    <div className="space-y-3">
      <PageHeader title="Atención" subtitle="Lo que espera una decisión tuya" />
      {(t3.isPending || blocked.isPending) && <Notice loading />}
      <ErrorText error={error} />
      {down.map((s) => <p key={s.instance_id} className="text-sm text-slate-500 dark:text-muted">Sin datos de {envName(s.environment_id)}: instancia no disponible.</p>)}
      {threads.map((t) => (
        <Card key={t.id} className="space-y-2">
          <ThreadLink url={t.url} environmentId={t.t3_environment_id} threadId={t.id} className="block space-y-1">
            <div className="font-medium break-words">{t.title}</div>
            <div className="flex flex-wrap gap-1"><Badge>{envName(t.status_.environment_id)}</Badge><ThreadStatus status={t.status} attention={t.attention} /><Badge>{t.model}</Badge></div>
          </ThreadLink>
          <ThreadReply id={t.id} />
        </Card>
      ))}
      {toValidate.map((i) => (
        <Card key={i.id} className="flex flex-wrap items-center gap-2">
          <StatusPill tone="violet">Validando</StatusPill>
          {i.project_id ? <Link to="/projects/$id" params={{ id: i.project_id }} className="min-w-0 flex-1 font-medium break-words">{i.title}</Link> : <span className="min-w-0 flex-1 font-medium break-words">{i.title}</span>}
          {i.progress && <p className="w-full text-xs text-slate-500 dark:text-muted">{i.progress}</p>}
        </Card>
      ))}
      {stuck.map((i) => (
        <Card key={i.id} className="border-l-4 border-l-red-500 dark:border-l-red-500">
          {i.project_id ? <Link to="/projects/$id" params={{ id: i.project_id }} className="font-medium break-words">{i.title}</Link> : <span className="font-medium break-words">{i.title}</span>}
          <p className="text-xs text-red-600 dark:text-red-400">{i.blocked_reason}</p>
        </Card>
      ))}
      {t3.isSuccess && blocked.isSuccess && validating.isSuccess && threads.length + stuck.length + toValidate.length === 0 && <Notice>Nada pendiente. Todo en orden.</Notice>}
    </div>
  );
}
