import { useState } from "react";
import { useMutation } from "@tanstack/react-query";
import { api, type T3ThreadContext } from "../api/client";
import { Button, ErrorText, Textarea } from "./ui";

type Pending = NonNullable<T3ThreadContext["pending"]>;

// PendingRequest answers the open request of a thread: structured questions
// with options and a free answer, or a single approval.
export function PendingRequest({ threadId, pending, onDone }: { threadId: string; pending: Pending; onDone: () => void }) {
  const [answers, setAnswers] = useState<Record<string, string>>({});
  const [custom, setCustom] = useState<Record<string, boolean>>({});
  const respond = useMutation({
    mutationFn: (body: Parameters<typeof api.t3Respond>[2]) => api.t3Respond(threadId, pending.id, body),
    onSuccess: () => { setAnswers({}); setCustom({}); onDone(); },
  });
  const questions = pending.questions ?? [];
  if (pending.kind !== "user_input") {
    return (
      <div className="space-y-2">
        <p className="text-sm font-medium">Permiso solicitado: {pending.kind}</p>
        {pending.detail
          ? <pre className="overflow-x-auto rounded bg-slate-100 dark:bg-raised p-2 text-xs">{pending.detail}</pre>
          : <p className="text-sm text-slate-500 dark:text-muted">T3 no expone la acción exacta: revísala en T3.</p>}
        <ErrorText error={respond.error} />
        <div className="flex gap-2">
          <Button disabled={respond.isPending || !pending.detail} onClick={() => respond.mutate({ decision: "accept" })}>Aprobar</Button>
          <Button variant="danger" disabled={respond.isPending} onClick={() => respond.mutate({ decision: "decline" })}>Rechazar</Button>
        </div>
      </div>
    );
  }
  return (
    <div className="space-y-2">
      {questions.map((q) => (
        <div key={q.id} className="space-y-1">
          <p className="text-sm font-medium">{q.question}</p>
          <div className="flex flex-wrap gap-1">
            {q.options.map((o) => (
              <Button key={o.label} variant={!custom[q.id] && answers[q.id] === o.label ? "default" : "ghost"} title={o.description} onClick={() => { setCustom({ ...custom, [q.id]: false }); setAnswers({ ...answers, [q.id]: o.label }); }}>{o.label}</Button>
            ))}
            {q.options.length > 0 && (
              <Button variant={custom[q.id] ? "default" : "ghost"} onClick={() => { setCustom({ ...custom, [q.id]: true }); setAnswers({ ...answers, [q.id]: "" }); }}>Otra respuesta</Button>
            )}
          </div>
          {(custom[q.id] || q.options.length === 0) && (
            <Textarea value={answers[q.id] ?? ""} onChange={(e) => setAnswers({ ...answers, [q.id]: e.target.value })} placeholder="Escribe tu respuesta" />
          )}
        </div>
      ))}
      <ErrorText error={respond.error} />
      <Button disabled={respond.isPending || questions.length === 0 || questions.some((q) => !answers[q.id]?.trim())} onClick={() => respond.mutate({ answers })}>Enviar respuesta</Button>
    </div>
  );
}
