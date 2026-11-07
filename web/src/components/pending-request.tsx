import { useEffect, useState } from "react";
import { useMutation } from "@tanstack/react-query";
import { CaretLeft, Check } from "@phosphor-icons/react";
import { api, type T3ThreadContext } from "../api/client";
import { ErrorText, Textarea } from "./ui";

type Pending = NonNullable<T3ThreadContext["pending"]>;
type Question = NonNullable<Pending["questions"]>[number];
type Answer = string | string[];

const primary = "inline-flex h-8 items-center rounded-full bg-slate-900 px-4 text-sm font-medium text-white transition hover:bg-slate-700 disabled:bg-slate-200 disabled:text-slate-400 dark:bg-white dark:text-slate-900 dark:hover:bg-slate-200 dark:disabled:bg-raised dark:disabled:text-muted";
const secondary = "inline-flex h-8 items-center rounded-full px-3 text-sm font-medium text-slate-600 transition hover:bg-slate-100 disabled:opacity-50 dark:text-text-secondary dark:hover:bg-raised";

// Mark is the radio (single choice) or checkbox (multiple choice) of an option.
function Mark({ multi, on }: { multi: boolean; on: boolean }) {
  const base = "grid size-4 shrink-0 place-items-center border transition-colors";
  if (multi) return <span className={`${base} rounded ${on ? "border-slate-900 bg-slate-900 text-white dark:border-white dark:bg-white dark:text-slate-900" : "border-slate-300 dark:border-border-strong"}`}>{on && <Check weight="bold" className="size-3" />}</span>;
  return <span className={`${base} rounded-full ${on ? "border-slate-900 dark:border-white" : "border-slate-300 dark:border-border-strong"}`}>{on && <span className="size-2 rounded-full bg-slate-900 dark:bg-white" />}</span>;
}

// PendingRequest answers the open request of a thread one question at a time:
// a single choice answers on click, multiple choice uses checkboxes. A free
// answer and the next button live in the thread composer when it passes
// onControl; without it a small text box and the buttons show here instead.
export type QuestionControl = { next: (text: string) => void; picked: boolean; last: boolean; busy: boolean };

export function PendingRequest({ threadId, pending, onDone, onControl }: { threadId: string; pending: Pending; onDone: () => void; onControl?: (c: QuestionControl | null) => void }) {
  const questions = pending.questions ?? [];
  const [step, setStep] = useState(0);
  const [answers, setAnswers] = useState<Record<string, Answer>>({});
  const [draft, setDraft] = useState("");
  const respond = useMutation({
    mutationFn: (body: Parameters<typeof api.t3Respond>[2]) => api.t3Respond(threadId, pending.id, body),
    onSuccess: () => { setAnswers({}); setStep(0); onDone(); },
  });
  const q: Question | undefined = questions[step];
  const last = step === questions.length - 1;

  // answer stores this question's answer and moves on, sending after the last one.
  const answer = (value: Answer) => {
    if (!q) return;
    const next = { ...answers, [q.id]: value };
    setAnswers(next);
    setDraft("");
    if (last) respond.mutate({ answers: next });
    else setStep(step + 1);
  };

  const pickedNow = q ? answers[q.id] : undefined;
  const hasPicks = Array.isArray(pickedNow) && pickedNow.length > 0;
  // The composer's next button: typed text wins, else the checked options.
  useEffect(() => {
    if (!onControl || pending.kind !== "user_input" || !q) return;
    onControl({ next: (text) => (text ? answer(text) : hasPicks && answer(pickedNow as string[])), picked: hasPicks, last, busy: respond.isPending });
  });
  useEffect(() => () => onControl?.(null), []); // eslint-disable-line react-hooks/exhaustive-deps

  if (pending.kind !== "user_input") {
    return (
      <div className="space-y-3">
        <p className="text-sm font-medium text-slate-900 dark:text-white">Permiso solicitado: {pending.kind}</p>
        {pending.detail
          ? <pre className="max-h-48 overflow-auto rounded-xl bg-slate-50 p-3 font-mono text-xs dark:bg-raised">{pending.detail}</pre>
          : <p className="text-sm text-slate-500 dark:text-muted">T3 no expone la acción exacta: revísala en T3.</p>}
        <ErrorText error={respond.error} />
        <div className="flex justify-end gap-2">
          <button type="button" className={secondary} disabled={respond.isPending} onClick={() => respond.mutate({ decision: "decline" })}>Rechazar</button>
          <button type="button" className={primary} disabled={respond.isPending || !pending.detail} onClick={() => respond.mutate({ decision: "accept" })}>Aprobar</button>
        </div>
      </div>
    );
  }
  if (!q) return null;

  const picked = Array.isArray(answers[q.id]) ? (answers[q.id] as string[]) : typeof answers[q.id] === "string" ? [answers[q.id] as string] : [];
  const toggle = (label: string) => setAnswers({ ...answers, [q.id]: picked.includes(label) ? picked.filter((x) => x !== label) : [...picked, label] });

  return (
    <div className="space-y-2">
      <div className="flex items-start gap-2">
        <p className="min-w-0 flex-1 text-sm font-medium text-slate-900 dark:text-white">{q.question}</p>
        {questions.length > 1 && <span className="shrink-0 pt-0.5 text-xs tabular-nums text-slate-400">{step + 1}/{questions.length}</span>}
      </div>
      <div role={q.multi_select ? "group" : "radiogroup"} className="-mx-2 space-y-0.5">
        {q.options.map((o) => {
          const on = picked.includes(o.label);
          return (
            <button key={o.label} type="button" role={q.multi_select ? "checkbox" : "radio"} aria-checked={on} disabled={respond.isPending}
              onClick={() => (q.multi_select ? toggle(o.label) : answer(o.label))}
              // Enter always moves on (Space still toggles a checkbox).
              onKeyDown={(e) => {
                if (e.key !== "Enter") return;
                e.preventDefault();
                if (!q.multi_select) answer(o.label);
                else if (picked.length) answer(picked);
              }}
              className="flex w-full items-start gap-2 rounded-lg px-2 py-2 text-left transition-colors hover:bg-slate-100 dark:hover:bg-raised">
              <span className="pt-0.5"><Mark multi={q.multi_select} on={on} /></span>
              <span className="min-w-0">
                <span className="block text-sm text-slate-900 dark:text-white">{o.label}</span>
                {o.description && <span className="block text-xs text-slate-500 dark:text-muted">{o.description}</span>}
              </span>
            </button>
          );
        })}
      </div>
      {!onControl && (
        <Textarea rows={2} value={draft} onChange={(e) => setDraft(e.target.value)} placeholder="O escribe tu respuesta" className="!rounded-xl" />
      )}
      <ErrorText error={respond.error} />
      <div className="flex items-center gap-2">
        {step > 0 && <button type="button" className={secondary} onClick={() => setStep(step - 1)}><CaretLeft weight="regular" className="mr-1 size-3.5" />Anterior</button>}
        <span className="ml-auto" />
        {!onControl && draft.trim() && <button type="button" className={primary} disabled={respond.isPending} onClick={() => answer(draft.trim())}>{last ? "Responder" : "Siguiente"}</button>}
        {!onControl && q.multi_select && <button type="button" className={primary} disabled={respond.isPending || picked.length === 0} onClick={() => answer(picked)}>{last ? "Responder" : "Siguiente"}</button>}
      </div>
    </div>
  );
}
