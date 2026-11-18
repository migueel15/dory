import { useState } from "react";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { CaretLeft, Check, Question as QuestionIcon } from "@phosphor-icons/react";
import { api, type Message } from "../api/client";
import { ErrorText, Textarea } from "./ui";
import { askQuestions, isAsk } from "./tool-card";

// QuestionCard is a question the agent asked through Dory's ask_user, ported
// from the v1 pending-request form: one question at a time, a single choice
// answers on click, several choices use checkboxes, and a free answer always
// fits. The answers go back as the next user message. Once answered (or when
// a later message exists) it shows read-only.

const primary = "inline-flex h-8 items-center rounded-full bg-slate-900 px-4 text-sm font-medium text-white transition hover:bg-slate-700 disabled:bg-slate-200 disabled:text-slate-400 dark:bg-white dark:text-slate-900 dark:hover:bg-slate-200 dark:disabled:bg-raised dark:disabled:text-muted";
const secondary = "inline-flex h-8 items-center rounded-full px-3 text-sm font-medium text-slate-600 transition hover:bg-slate-100 disabled:opacity-50 dark:text-text-secondary dark:hover:bg-raised";

type Answer = string | string[];

function Mark({ multi, on }: { multi: boolean; on: boolean }) {
  const b = "grid size-4 shrink-0 place-items-center border transition-colors";
  if (multi) return <span className={`${b} rounded ${on ? "border-slate-900 bg-slate-900 text-white dark:border-white dark:bg-white dark:text-slate-900" : "border-slate-300 dark:border-border-strong"}`}>{on && <Check weight="bold" className="size-3" />}</span>;
  return <span className={`${b} rounded-full ${on ? "border-slate-900 dark:border-white" : "border-slate-300 dark:border-border-strong"}`}>{on && <span className="size-2 rounded-full bg-slate-900 dark:bg-white" />}</span>;
}

export function QuestionCard({ m, threadId, open }: { m: Message; threadId: string; open: boolean }) {
  const qc = useQueryClient();
  const asked = isAsk(m) ? askQuestions(m) : m.tool?.questions ?? [];
  const questions = asked.length ? asked : [{ question: m.text }];
  const [step, setStep] = useState(0);
  const [answers, setAnswers] = useState<Answer[]>([]);
  const [draft, setDraft] = useState("");
  const send = useMutation({
    mutationFn: (all: Answer[]) => api.send(threadId, questions.map((q, i) => {
      const a = all[i];
      return `${questions.length > 1 ? `**${q.header || q.question}**\n` : ""}${Array.isArray(a) ? a.join(", ") : a}`;
    }).join("\n\n")),
    onSuccess: () => qc.invalidateQueries({ queryKey: ["thread", threadId] }),
  });
  const q = questions[step];
  const last = step === questions.length - 1;
  const multi = !!q.multi_select;
  const current = answers[step];
  const picked = Array.isArray(current) ? current : typeof current === "string" ? [current] : [];

  const answer = (value: Answer) => {
    const next = [...answers];
    next[step] = value;
    setAnswers(next);
    setDraft("");
    if (last) send.mutate(next);
    else setStep(step + 1);
  };
  const toggle = (label: string) => {
    const next = [...answers];
    next[step] = picked.includes(label) ? picked.filter((x) => x !== label) : [...picked, label];
    setAnswers(next);
  };

  const live = open && !send.isSuccess;
  return (
    <div className="rounded-2xl border border-slate-200 bg-white p-3.5 shadow-sm dark:border-border dark:bg-surface">
      <div className="mb-2 flex items-center gap-2 text-xs font-medium text-slate-500 dark:text-muted">
        <QuestionIcon className="size-4" />{live ? "El agente te pregunta" : "Pregunta del agente"}
        {q.header && <span className="rounded-md bg-slate-100 px-1.5 py-0.5 text-[11px] dark:bg-raised">{q.header}</span>}
        {questions.length > 1 && live && <span className="ml-auto tabular-nums">{step + 1}/{questions.length}</span>}
      </div>
      {!live ? (
        <ul className="space-y-1 text-sm text-slate-700 dark:text-text-secondary">{questions.map((x, i) => <li key={i}>{x.question}</li>)}</ul>
      ) : (
        <div className="space-y-2">
          <p className="text-sm font-medium text-slate-900 dark:text-white">{q.question}</p>
          {!!q.options?.length && (
            <div role={multi ? "group" : "radiogroup"} className="-mx-2 space-y-0.5">
              {q.options.map((o) => {
                const on = picked.includes(o.label);
                return (
                  <button key={o.label} type="button" role={multi ? "checkbox" : "radio"} aria-checked={on} disabled={send.isPending}
                    onClick={() => (multi ? toggle(o.label) : answer(o.label))}
                    onKeyDown={(e) => { if (e.key !== "Enter") return; e.preventDefault(); if (!multi) answer(o.label); else if (picked.length) answer(picked); }}
                    className="flex w-full items-start gap-2 rounded-lg px-2 py-2 text-left transition-colors hover:bg-slate-100 dark:hover:bg-raised">
                    <span className="pt-0.5"><Mark multi={multi} on={on} /></span>
                    <span className="min-w-0">
                      <span className="block text-sm text-slate-900 dark:text-white">{o.label}</span>
                      {o.description && <span className="block text-xs text-slate-500 dark:text-muted">{o.description}</span>}
                    </span>
                  </button>
                );
              })}
            </div>
          )}
          <Textarea rows={2} value={draft} onChange={(e) => setDraft(e.target.value)} placeholder={q.options?.length ? "O escribe tu respuesta" : "Tu respuesta"} className="!rounded-xl"
            onKeyDown={(e) => { if (e.key === "Enter" && !e.shiftKey && draft.trim()) { e.preventDefault(); answer(draft.trim()); } }} />
          <ErrorText error={send.error} />
          <div className="flex items-center gap-2">
            {step > 0 && <button type="button" className={secondary} onClick={() => setStep(step - 1)}><CaretLeft className="mr-1 size-3.5" />Anterior</button>}
            <span className="ml-auto" />
            {draft.trim() ? <button type="button" className={primary} disabled={send.isPending} onClick={() => answer(draft.trim())}>{last ? "Responder" : "Siguiente"}</button>
              : multi && <button type="button" className={primary} disabled={send.isPending || picked.length === 0} onClick={() => answer(picked)}>{last ? "Responder" : "Siguiente"}</button>}
          </div>
        </div>
      )}
    </div>
  );
}
