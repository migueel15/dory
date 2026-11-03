import { ThreadLink } from "../components/thread-link";
import { useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Link } from "@tanstack/react-router";
import { api } from "../api/client";
import { Badge, Button, Card, ErrorText, Textarea } from "../components/ui";
import { useProject, useProjectThreads } from "../project";

export const useT3 = (enabled = true) => useQuery({ queryKey: ["t3-status"], queryFn: api.t3Status, refetchInterval: 10_000, enabled });
export const useBlocked = (enabled = true) => useQuery({ queryKey: ["items", "blocked"], queryFn: () => api.items.list({ status: "blocked" }), enabled });

// useAttention lists the threads waiting for the user and the blocked items,
// limited to the selected project (threads count when linked to its items).
function useAttention(enabled = true) {
  const project = useProject();
  const t3 = useT3(enabled);
  const blocked = useBlocked(enabled);
  const linked = useProjectThreads(enabled ? project : "");
  const threads = t3.data?.flatMap((s) => s.threads.filter((t) => t.attention && (!linked.keys || linked.keys.has(`${s.instance_id}:${t.id}`))).map((t) => ({ ...t, status_: s }))) ?? [];
  const stuck = blocked.data?.filter((i) => !project || i.project_id === project) ?? [];
  return { t3, blocked, threads, stuck, error: t3.error ?? blocked.error ?? linked.error };
}

export function useAttentionCount(enabled: boolean) {
  const { threads, stuck } = useAttention(enabled);
  return threads.length + stuck.length;
}

function ThreadReply({ id }: { id: string }) {
  const [open, setOpen] = useState(false);
  const [text, setText] = useState("");
  const [answers, setAnswers] = useState<Record<string, string>>({});
  const qc = useQueryClient();
  const ctx = useQuery({ queryKey: ["t3-context", id], queryFn: () => api.t3Context(id), enabled: open, refetchInterval: 5_000 });
  const refresh = () => {
    qc.invalidateQueries({ queryKey: ["t3-context", id] });
    qc.invalidateQueries({ queryKey: ["t3-status"] });
  };
  const send = useMutation({ mutationFn: () => api.t3Send(id, text), onSuccess: () => { setText(""); refresh(); } });
  const respond = useMutation({
    mutationFn: (body: Parameters<typeof api.t3Respond>[2]) => api.t3Respond(id, ctx.data!.pending!.id, body),
    onSuccess: () => { setAnswers({}); refresh(); },
  });
  if (!open) return <Button variant="ghost" onClick={() => setOpen(true)}>Responder</Button>;
  const pending = ctx.data?.pending;
  const questions = pending?.questions ?? [];
  return (
    <div className="space-y-2">
      <ErrorText error={ctx.error ?? send.error ?? respond.error} />
      <div className="space-y-1 text-sm">
        {ctx.data?.messages.map((m, i) => (
          <p key={i} className="break-words whitespace-pre-wrap"><span className="text-slate-500 dark:text-slate-400">{m.role}: </span>{m.text}</p>
        ))}
      </div>
      {pending && pending.kind === "user_input" && (
        <div className="space-y-2">
          {questions.map((q) => (
            <div key={q.id} className="space-y-1">
              <p className="text-sm font-medium">{q.question}</p>
              <div className="flex flex-wrap gap-1">
                {q.options.map((o) => (
                  <Button key={o.label} variant={answers[q.id] === o.label ? "default" : "ghost"} title={o.description} onClick={() => setAnswers({ ...answers, [q.id]: o.label })}>{o.label}</Button>
                ))}
              </div>
            </div>
          ))}
          <Button disabled={respond.isPending || questions.length === 0 || questions.some((q) => !answers[q.id])} onClick={() => respond.mutate({ answers })}>Enviar respuesta</Button>
        </div>
      )}
      {pending && pending.kind !== "user_input" && (
        <div className="space-y-2">
          <p className="text-sm font-medium">Permiso solicitado: {pending.kind}</p>
          {pending.detail
            ? <pre className="overflow-x-auto rounded bg-slate-100 dark:bg-slate-800 p-2 text-xs">{pending.detail}</pre>
            : <p className="text-sm text-slate-500 dark:text-slate-400">T3 no expone la acción exacta: revísala en T3.</p>}
          <div className="flex gap-2">
            <Button disabled={respond.isPending || !pending.detail} onClick={() => respond.mutate({ decision: "accept" })}>Aprobar</Button>
            <Button variant="danger" disabled={respond.isPending} onClick={() => respond.mutate({ decision: "decline" })}>Rechazar</Button>
          </div>
        </div>
      )}
      <Textarea value={text} onChange={(e) => setText(e.target.value)} placeholder="Mensaje para el agente" />
      <Button disabled={send.isPending || !text.trim()} onClick={() => send.mutate()}>Enviar mensaje</Button>
    </div>
  );
}

export function Attention() {
  const { t3, blocked, threads, stuck, error } = useAttention();
  const envs = useQuery({ queryKey: ["environments"], queryFn: () => api.environments.list() });
  const down = t3.data?.filter((s) => s.state !== "available") ?? [];
  const envName = (id: string) => envs.data?.find((e) => e.id === id)?.name ?? "";
  return (
    <div className="space-y-3">
      <h1 className="text-xl font-bold">Atención</h1>
      <ErrorText error={error} />
      {down.map((s) => <p key={s.instance_id} className="text-sm text-slate-500 dark:text-slate-400">Sin datos de {envName(s.environment_id)}: instancia no disponible.</p>)}
      {threads.map((t) => (
        <Card key={t.id} className="space-y-2">
          <ThreadLink url={t.url} environmentId={t.t3_environment_id} threadId={t.id} className="block space-y-1">
            <div className="font-medium break-words">{t.title}</div>
            <div className="flex flex-wrap gap-1"><Badge>{envName(t.status_.environment_id)}</Badge><Badge>{t.status}</Badge><Badge>{t.model}</Badge></div>
          </ThreadLink>
          <ThreadReply id={t.id} />
        </Card>
      ))}
      {stuck.map((i) => (
        <Card key={i.id}>
          {i.project_id ? <Link to="/projects/$id" params={{ id: i.project_id }} className="font-medium break-words">{i.title}</Link> : <span className="font-medium break-words">{i.title}</span>}
          <p className="text-xs text-red-600 dark:text-red-400">{i.blocked_reason}</p>
        </Card>
      ))}
      {t3.isSuccess && blocked.isSuccess && threads.length + stuck.length === 0 && <p className="text-sm text-slate-500 dark:text-slate-400">Nada pendiente.</p>}
    </div>
  );
}
