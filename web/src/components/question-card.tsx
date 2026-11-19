import { useState } from "react";
import { CaretLeft, CaretRight, Check, Question as QuestionIcon } from "@phosphor-icons/react";
import { type Message, type Question } from "../api/client";
import { ErrorText } from "./ui";
import { askQuestions, isAsk } from "./tool-card";

// The agent's questions are answered from the composer: QuestionPanel sits
// inside it, one question at a time, and its text box gives the free answer.
// A question the harness asks natively (Claude's AskUserQuestion, Codex's
// requestUserInput) pauses the turn and the answers resolve its tool call.
// One asked through Dory's ask_user is answered with the next user message.
// The chat only shows the questions read-only, with their answers.

const primary = "inline-flex h-8 items-center rounded-full bg-slate-900 px-4 text-sm font-medium text-white transition hover:bg-slate-700 disabled:bg-slate-200 disabled:text-slate-400 dark:bg-white dark:text-slate-900 dark:hover:bg-slate-200 dark:disabled:bg-raised dark:disabled:text-muted";
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
      {q?.header && <span className="rounded-md bg-slate-100 px-1.5 py-0.5 text-[0.6875rem] dark:bg-raised">{q.header}</span>}
      {total !== undefined && total > 1 && <span className="ml-auto tabular-nums">{(step ?? 0) + 1}/{total}</span>}
    </div>
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

// openAsk finds a question asked through Dory's ask_user that is still
// waiting: the call is running and nothing the user sent came after it. An
// answered call has finished, since ask_user returns the answers itself.
export function openAsk(messages: Message[]): { id: string; questions: Question[] } | undefined {
  const viaTool = messages.some(isAsk);
  let open: Message | undefined;
  for (const m of messages) open = m.role === "user" ? undefined : isAsk(m) ? (m.status === "running" ? m : undefined) : (!viaTool && m.role === "question") ? m : open;
  if (!open) return undefined;
  const asked = isAsk(open) ? askQuestions(open) : open.tool?.questions ?? [];
  return { id: open.id, questions: asked.length ? asked : [{ question: open.text }] };
}

// askAnswerText writes the answers to an ask_user call as one user message.
export const askAnswerText = (questions: Question[], answers: string[][]) =>
  questions.map((q, i) => `${questions.length > 1 ? `**${q.header || q.question}**\n` : ""}${answers[i].join(", ")}`).join("\n\n");

// QuestionFlow walks the questions of a pending request one by one. The
// composer owns it: its text box writes the free answer of the current one.
export type QuestionFlow = ReturnType<typeof useQuestionFlow>;

export function useQuestionFlow(id: string, questions: Question[], onDone: (answers: string[][], files: string[][]) => void) {
  const [state, setState] = useState<{ id: string; step: number; answers: Answer[]; files: string[][] }>({ id, step: 0, answers: [], files: [] });
  const { step, answers, files } = state.id === id ? state : { step: 0, answers: [] as Answer[], files: [] as string[][] };
  const q: Question | undefined = questions[step];
  const last = step === questions.length - 1;
  const current = answers[step];
  const picked = Array.isArray(current) ? current : typeof current === "string" ? [current] : [];
  const go = (to: number, next = answers, nextFiles = files) => setState({ id, step: to, answers: next, files: nextFiles });
  // answer sets the answer of the current question, with the ids of the files
  // sent along with a free answer, and moves on; the last one sends them all.
  const answer = (value: Answer, attached: string[] = []) => {
    const next = [...answers];
    next[step] = value;
    const nextFiles = [...files];
    nextFiles[step] = attached;
    if (last) {
      setState({ id, step, answers: next, files: nextFiles });
      onDone(questions.map((_, i) => { const a = next[i]; return Array.isArray(a) ? a : a ? [a] : []; }), questions.map((_, i) => nextFiles[i] ?? []));
    } else go(step + 1, next, nextFiles);
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
        {q.header && <span className="rounded-md bg-slate-100 px-1.5 py-0.5 text-[0.6875rem] dark:bg-raised">{q.header}</span>}
        {total > 1 && (
          <span className="ml-auto flex items-center gap-0.5">
            <button type="button" className={nav} aria-label="Pregunta anterior" title="Anterior" disabled={step === 0 || busy} onClick={flow.back}><CaretLeft className="size-4" /></button>
            <button type="button" className={nav} aria-label="Pregunta siguiente" title="Siguiente" disabled={!flow.canForward || busy} onClick={flow.forward}><CaretRight className="size-4" /></button>
          </span>
        )}
      </div>
      <p className="mb-1.5 text-[0.9375rem] font-medium text-slate-900 dark:text-white">{q.question}</p>
      {!!q.options?.length && (
        <div role={multi ? "group" : "radiogroup"} aria-label={q.question} className="-mx-2 space-y-0.5">
          {q.options.map((o, i) => {
            const on = picked.includes(o.label);
            return (
              <button key={o.label} type="button" role={multi ? "checkbox" : "radio"} aria-checked={on} disabled={busy}
                onClick={() => (multi ? flow.toggle(o.label) : flow.answer(o.label))}
                className={`flex w-full items-start gap-2.5 rounded-lg px-2 py-2 text-left transition-colors hover:bg-slate-100 dark:hover:bg-raised ${on ? "bg-slate-100 dark:bg-raised" : ""}`}>
                <span className="grid size-5 shrink-0 place-items-center rounded-md border border-slate-300 text-[0.6875rem] tabular-nums text-slate-500 dark:border-border-strong dark:text-muted">{i + 1}</span>
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
// harnesses without native questions). The chat shows it read-only; the
// composer answers it and the answers arrive as the next user message.
export function QuestionCard({ m }: { m: Message }) {
  const asked = isAsk(m) ? askQuestions(m) : m.tool?.questions ?? [];
  const questions: Question[] = asked.length ? asked : [{ question: m.text }];
  return (
    <div className={card}>
      <Header text="Pregunta del agente" q={questions.length === 1 ? questions[0] : undefined} />
      <AnsweredQuestions questions={questions} />
    </div>
  );
}
