import { useState } from "react";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { CaretLeft, CaretRight, Check, Question as QuestionIcon } from "@phosphor-icons/react";
import { api, type Message, type PendingRequest, type Question } from "../api/client";
import { ErrorText, Textarea } from "./ui";
import { askQuestions, isAsk } from "./tool-card";

// The agent's questions, ported from the v1 pending-request form: one
// question at a time, a single choice answers on click, several choices use
// checkboxes, and a free answer always fits.
//
// A question the harness asks natively (Claude's AskUserQuestion, Codex's
// requestUserInput) pauses the turn: QuestionPanel sits inside the composer and the
// answers go back as the result of the tool, so the chat only shows the card
// with the questions and their answers. Older threads asked through Dory's
// ask_user, whose answer went back as the next user message; QuestionCard
// keeps that path.

const primary = "inline-flex h-8 items-center rounded-full bg-slate-900 px-4 text-sm font-medium text-white transition hover:bg-slate-700 disabled:bg-slate-200 disabled:text-slate-400 dark:bg-white dark:text-slate-900 dark:hover:bg-slate-200 dark:disabled:bg-raised dark:disabled:text-muted";
const secondary = "inline-flex h-8 items-center rounded-full px-3 text-sm font-medium text-slate-600 transition hover:bg-slate-100 disabled:opacity-50 dark:text-text-secondary dark:hover:bg-raised";
const card = "rounded-2xl border border-slate-200 bg-white p-3.5 shadow-sm dark:border-border dark:bg-surface";

type Answer = string | string[];

function Mark({ multi, on }: { multi: boolean; on: boolean }) {
  const b = "grid size-4 shrink-0 place-items-center border transition-colors";
  if (multi) return <span className={`${b} rounded ${on ? "border-slate-900 bg-slate-900 text-white dark:border-white dark:bg-white dark:text-slate-900" : "border-slate-300 dark:border-border-strong"}`}>{on && <Check weight="bold" className="size-3" />}</span>;
  return <span className={`${b} rounded-full ${on ? "border-slate-900 dark:border-white" : "border-slate-300 dark:border-border-strong"}`}>{on && <span className="size-2 rounded-full bg-slate-900 dark:bg-white" />}</span>;
}

function Header({ text, q, step, total }: { text: string; q?: Question; step?: number; total?: number }) {
  return (
    <div className="mb-2 flex items-center gap-2 text-xs font-medium text-slate-500 dark:text-muted">
      <QuestionIcon className="size-4" />{text}
      {q?.header && <span className="rounded-md bg-slate-100 px-1.5 py-0.5 text-[11px] dark:bg-raised">{q.header}</span>}
      {total !== undefined && total > 1 && <span className="ml-auto tabular-nums">{(step ?? 0) + 1}/{total}</span>}
    </div>
  );
}

// QuestionForm asks the questions one by one and hands back every answer.
function QuestionForm({ questions, onSubmit, busy, error }: { questions: Question[]; onSubmit: (answers: string[][]) => void; busy: boolean; error: unknown }) {
  const [step, setStep] = useState(0);
  const [answers, setAnswers] = useState<Answer[]>([]);
  const [draft, setDraft] = useState("");
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
    if (last) onSubmit(questions.map((_, i) => { const a = next[i]; return Array.isArray(a) ? a : a ? [a] : []; }));
    else setStep(step + 1);
  };
  const toggle = (label: string) => {
    const next = [...answers];
    next[step] = picked.includes(label) ? picked.filter((x) => x !== label) : [...picked, label];
    setAnswers(next);
  };

  return (
    <>
      <Header text="El agente te pregunta" q={q} step={step} total={questions.length} />
      <div className="space-y-2">
        <p className="text-sm font-medium text-slate-900 dark:text-white">{q.question}</p>
        {!!q.options?.length && (
          <div role={multi ? "group" : "radiogroup"} className="-mx-2 space-y-0.5">
            {q.options.map((o) => {
              const on = picked.includes(o.label);
              return (
                <button key={o.label} type="button" role={multi ? "checkbox" : "radio"} aria-checked={on} disabled={busy}
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
        <ErrorText error={error} />
        <div className="flex items-center gap-2">
          {step > 0 && <button type="button" className={secondary} onClick={() => setStep(step - 1)}><CaretLeft className="mr-1 size-3.5" />Anterior</button>}
          <span className="ml-auto" />
          {draft.trim() ? <button type="button" className={primary} disabled={busy} onClick={() => answer(draft.trim())}>{last ? "Responder" : "Siguiente"}</button>
            : multi && <button type="button" className={primary} disabled={busy || picked.length === 0} onClick={() => answer(picked)}>{last ? "Responder" : "Siguiente"}</button>}
        </div>
      </div>
    </>
  );
}

// AnsweredQuestions lists the questions with the answers given, if any.
function AnsweredQuestions({ questions, waiting }: { questions: Question[]; waiting?: boolean }) {
  return (
    <ul className="space-y-2 text-sm">
      {questions.map((x, i) => (
        <li key={i}>
          <p className="text-slate-700 dark:text-text-secondary">{x.question}</p>
          {x.answer?.length ? <p className="mt-0.5 flex items-start gap-1.5 font-medium text-slate-900 dark:text-white"><Check weight="bold" className="mt-1 size-3.5 shrink-0" />{x.answer.join(", ")}</p>
            : waiting && <p className="mt-0.5 text-xs text-slate-500 dark:text-muted">Esperando tu respuesta</p>}
        </li>
      ))}
    </ul>
  );
}

// NativeQuestionCard is a question the harness asked as a tool: the chat
// shows it read-only, with the answers once they are in.
export function NativeQuestionCard({ m }: { m: Message }) {
  const questions = m.tool?.questions ?? [];
  const answered = questions.some((q) => q.answer?.length);
  return (
    <div className={card}>
      <Header text="Pregunta del agente" q={questions.length === 1 ? questions[0] : undefined} />
      <AnsweredQuestions questions={questions} waiting={!answered && m.status === "running"} />
    </div>
  );
}

// QuestionFlow walks the questions of a pending request one by one. The
// composer owns it: its text box writes the free answer of the current one.
export type QuestionFlow = ReturnType<typeof useQuestionFlow>;

export function useQuestionFlow(id: string, questions: Question[], onDone: (answers: string[][]) => void) {
  const [state, setState] = useState<{ id: string; step: number; answers: Answer[] }>({ id, step: 0, answers: [] });
  const { step, answers } = state.id === id ? state : { step: 0, answers: [] as Answer[] };
  const q: Question | undefined = questions[step];
  const last = step === questions.length - 1;
  const current = answers[step];
  const picked = Array.isArray(current) ? current : typeof current === "string" ? [current] : [];
  const go = (to: number, next = answers) => setState({ id, step: to, answers: next });
  const answer = (value: Answer) => {
    const next = [...answers];
    next[step] = value;
    if (last) {
      setState({ id, step, answers: next });
      onDone(questions.map((_, i) => { const a = next[i]; return Array.isArray(a) ? a : a ? [a] : []; }));
    } else go(step + 1, next);
  };
  const toggle = (label: string) => {
    const next = [...answers];
    next[step] = picked.includes(label) ? picked.filter((x) => x !== label) : [...picked, label];
    go(step, next);
  };
  return {
    q, step, total: questions.length, last, picked, multi: !!q?.multi_select, answer, toggle,
    // An answered question can be revisited; the next one opens once this one has an answer.
    back: () => step > 0 && go(step - 1),
    forward: () => !last && current !== undefined && go(step + 1),
    canForward: !last && current !== undefined,
  };
}

// QuestionPanel shows the current question on top of the composer, joined to
// it. Options answer on click (or with their number); the composer's text box
// gives any other answer.
export function QuestionPanel({ flow, busy, error }: { flow: QuestionFlow; busy: boolean; error: unknown }) {
  const { q, step, total, multi, picked, last } = flow;
  if (!q) return null;
  const nav = "grid size-7 place-items-center rounded-full text-slate-500 transition hover:bg-slate-100 disabled:opacity-30 dark:text-muted dark:hover:bg-raised";
  return (
    <div className="max-h-[45vh] overflow-y-auto border-b border-slate-200 px-4 pb-3 pt-3 dark:border-border"
      onKeyDown={(e) => {
        const n = Number(e.key);
        if (!busy && n >= 1 && n <= (q.options?.length ?? 0) && !(e.target instanceof HTMLTextAreaElement)) {
          e.preventDefault();
          const label = q.options![n - 1].label;
          if (multi) flow.toggle(label); else flow.answer(label);
        }
      }}>
      <div className="mb-2 flex items-center gap-2 text-xs font-medium text-slate-500 dark:text-muted">
        <QuestionIcon className="size-4" />
        <span>{total > 1 ? `Pregunta ${step + 1} de ${total}` : "El agente te pregunta"}</span>
        {q.header && <span className="rounded-md bg-slate-100 px-1.5 py-0.5 text-[11px] dark:bg-raised">{q.header}</span>}
        {total > 1 && (
          <span className="ml-auto flex items-center gap-0.5">
            <button type="button" className={nav} aria-label="Pregunta anterior" title="Anterior" disabled={step === 0 || busy} onClick={flow.back}><CaretLeft className="size-4" /></button>
            <button type="button" className={nav} aria-label="Pregunta siguiente" title="Siguiente" disabled={!flow.canForward || busy} onClick={flow.forward}><CaretRight className="size-4" /></button>
          </span>
        )}
      </div>
      <p className="mb-1.5 text-[15px] font-medium text-slate-900 dark:text-white">{q.question}</p>
      {!!q.options?.length && (
        <div role={multi ? "group" : "radiogroup"} aria-label={q.question} className="-mx-2 space-y-0.5">
          {q.options.map((o, i) => {
            const on = picked.includes(o.label);
            return (
              <button key={o.label} type="button" role={multi ? "checkbox" : "radio"} aria-checked={on} disabled={busy}
                onClick={() => (multi ? flow.toggle(o.label) : flow.answer(o.label))}
                className={`flex w-full items-start gap-2.5 rounded-lg px-2 py-2 text-left transition-colors hover:bg-slate-100 dark:hover:bg-raised ${on ? "bg-slate-100 dark:bg-raised" : ""}`}>
                <span className="grid size-5 shrink-0 place-items-center rounded-md border border-slate-300 text-[11px] tabular-nums text-slate-500 dark:border-border-strong dark:text-muted">{i + 1}</span>
                <span className="min-w-0 flex-1">
                  <span className="block text-sm text-slate-900 dark:text-white">{o.label}</span>
                  {o.description && <span className="block text-xs text-slate-500 dark:text-muted">{o.description}</span>}
                </span>
                {multi && <span className="pt-0.5"><Mark multi on={on} /></span>}
              </button>
            );
          })}
        </div>
      )}
      {multi && (
        <div className="mt-2 flex justify-end">
          <button type="button" className={primary} disabled={busy || picked.length === 0} onClick={() => flow.answer(picked)}>{last ? "Responder" : "Siguiente"}</button>
        </div>
      )}
      <ErrorText error={error} />
    </div>
  );
}

// QuestionCard is a question asked through Dory's ask_user (older threads and
// harnesses without native questions): the answers go back as the next user
// message. Once answered (or when a later message exists) it shows read-only.
export function QuestionCard({ m, threadId, open }: { m: Message; threadId: string; open: boolean }) {
  const qc = useQueryClient();
  const asked = isAsk(m) ? askQuestions(m) : m.tool?.questions ?? [];
  const questions: Question[] = asked.length ? asked : [{ question: m.text }];
  const send = useMutation({
    mutationFn: (all: string[][]) => api.send(threadId, questions.map((q, i) =>
      `${questions.length > 1 ? `**${q.header || q.question}**\n` : ""}${all[i].join(", ")}`).join("\n\n")),
    onSuccess: () => qc.invalidateQueries({ queryKey: ["thread", threadId] }),
  });
  const live = open && !send.isSuccess;
  return (
    <div className={card}>
      {live ? <QuestionForm questions={questions} onSubmit={(a) => send.mutate(a)} busy={send.isPending} error={send.error} />
        : <><Header text="Pregunta del agente" q={questions.length === 1 ? questions[0] : undefined} /><AnsweredQuestions questions={questions} /></>}
    </div>
  );
}
