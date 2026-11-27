import { Brain,CaretDown,CaretRight,ChatText,Question as QuestionIcon,TerminalWindow,Wrench } from "@phosphor-icons/react";
import { useEffect,useState } from "react";
import { type Message } from "../../api/client";
import { Md } from "../../components/markdown";
import { t } from "../../i18n/es";
import { htmlRef } from "./html-card";
import { NativeQuestionCard,QuestionCard } from "./question-card";
import { GroupCard,ToolCard,askQuestions,isAsk } from "./tool-card";
import { turnFiles,type Turn } from "./turn-changes";

export const running = (m: Message) => m.streaming || (m.role === "tool" && !["completed", "failed", "error", "declined", "interrupted", "cancelled"].includes(m.status ?? ""));
export const failed = (m: Message) => m.status === "failed" || m.status === "error";

// commandName is the program a shell command runs: "cd x && sed -i …" → "sed".
export const commandName = (cmd = "") => {
  for (const part of cmd.split(/&&|\|\||;|\n/)) {
    const words = part.trim().split(/\s+/).filter((w) => w && !/^\w+=/.test(w));
    if (words.length && words[0] !== "cd") return words[0].replace(/^.*\//, "");
  }
  return "";
};

export const shells = new Set(["Bash", "command execution", "exec_command", "shell", "bash"]);

// stepKind says which steps share a line: consecutive thoughts, commands,
// notes or calls to the same tool show as one line that updates in place.
// Calls in Dory's terms (m.tool) group only reads and searches; each other
// call is its own card.
export const stepKind = (m: Message) => m.role === "assistant" ? "note" : m.role === "reasoning" ? "think"
  : m.tool ? (m.tool.kind === "read" || m.tool.kind === "search" ? `group:${m.tool.kind}` : `call:${m.id}`)
  : shells.has(m.title ?? "") ? "cmd" : `tool:${m.title}`;

export function stepRuns(items: Message[]): Message[][] {
  const out: Message[][] = [];
  for (const m of items) {
    if (m.role === "reasoning" && !m.text.trim() && !m.streaming) continue;
    const last = out.at(-1);
    if (last && !isQuestion(m) && !isQuestion(last[0]) && stepKind(last[0]) === stepKind(m)) last.push(m);
    else out.push([m]);
  }
  return out;
}

export const isQuestion = (m: Message) => isAsk(m) || m.tool?.kind === "question" || m.role === "question";

// QuestionStep is a question of the agent among the steps: one line, and the
// card with the questions and answers when opened.
export function QuestionStep({ m }: { m: Message }) {
  const [open, setOpen] = useState(false);
  const qs = isAsk(m) ? askQuestions(m) : m.tool?.questions ?? [];
  const first = qs[0]?.question ?? m.text;
  const answered = qs.some((q) => q.answer?.length);
  return (
    <div className="text-[0.8125rem] leading-relaxed text-slate-500 dark:text-muted">
      <button type="button" onClick={() => setOpen(!open)} aria-expanded={open}
        className="flex w-full min-w-0 items-center gap-2 text-left hover:text-slate-900 dark:hover:text-white">
        <QuestionIcon weight="regular" className="size-4 shrink-0" />
        <span className="min-w-0 flex-1 truncate">{answered ? "Respondida" : t("question.title")}: {oneLine(first)}{qs.length > 1 ? ` (+${qs.length - 1})` : ""}</span>
      </button>
      {open && <div className="mb-1 ml-6 mt-1.5">{isAsk(m) || m.role === "question" ? <QuestionCard m={m} /> : <NativeQuestionCard m={m} />}</div>}
    </div>
  );
}

export const oneLine = (t = "") => t.replace(/[`*_#>]/g, "").replace(/\s+/g, " ").trim();

// StepLine is a run of steps of one kind: an icon and one line with the latest
// step. A click opens every step of the run in full.
export function StepLine({ run }: { run: Message[] }) {
  if (isQuestion(run[0])) return <QuestionStep m={run[0]} />;
  if (run[0].tool) return run[0].tool.kind === "read" || run[0].tool.kind === "search" ? <GroupCard run={run} /> : <ToolCard m={run[0]} />;
  return <LegacyStep run={run} />;
}

// LegacyStep draws steps saved before calls carried their Dory kind.
export function LegacyStep({ run }: { run: Message[] }) {
  const [open, setOpen] = useState(false);
  const m = run[run.length - 1];
  const kind = stepKind(m);
  const live = running(m), bad = failed(m);
  const icon = kind === "think" ? <Brain weight="regular" className="size-4 shrink-0" /> : kind === "cmd" ? <TerminalWindow weight="regular" className="size-4 shrink-0" />
    : kind === "note" ? <ChatText weight="regular" className="size-4 shrink-0" /> : <Wrench weight="regular" className="size-4 shrink-0" />;
  const line = kind === "think" ? (oneLine(m.text) || "Thinking…")
    : kind === "note" ? oneLine(m.text)
    : kind === "cmd" ? `${live ? "Running" : bad ? "Failed" : "Ran"} ${commandName(m.detail) || "command"}`
    : m.title || "Tool";
  const full = (x: Message) => x.role === "tool"
    ? <pre key={x.id} className="max-h-60 overflow-auto whitespace-pre-wrap break-all rounded bg-slate-100 p-2 font-mono text-[0.6875rem] dark:bg-raised">{[x.detail, x.text].filter(Boolean).join("\n\n")}</pre>
    : <div key={x.id} className="[&_*]:!text-inherit [&_p]:my-0.5"><Md>{x.text}</Md></div>;
  return (
    <div className={`text-[0.8125rem] leading-relaxed ${kind === "note" ? "text-slate-700 dark:text-text-secondary" : "text-slate-500 dark:text-muted"}`}>
      <button type="button" onClick={() => setOpen(!open)} aria-expanded={open}
        className="flex w-full min-w-0 items-center gap-2 text-left hover:text-slate-900 dark:hover:text-white">
        {icon}
        <span className={`min-w-0 flex-1 truncate ${live ? "motion-safe:animate-pulse" : ""} ${bad ? "text-red-600 dark:text-red-400" : ""}`}>{line}</span>
        {run.length > 1 && <span className="shrink-0 text-[0.6875rem] tabular-nums opacity-60">{run.length}</span>}
      </button>
      {open && <div className="mb-1 ml-6 mt-1 space-y-1.5">{run.map(full)}</div>}
    </div>
  );
}

// Worked is a turn's work folded into one entry.
export type Worked = { worked: true; id: string; items: Message[]; secs: number; live?: boolean; since?: string };

// layout lays a conversation out turn by turn. In a finished turn, everything
// between the user's message and the final answer folds into one "Worked
// for…" entry; the turn still running shows its steps as they come.
export type Changes = { changes: true; id: string; turn: Turn };

// changeTurns lists the turns that edited files, oldest first.
export const changeTurns = (ms: Message[]): Turn[] => layout(ms, false).flatMap((x) => ("changes" in x ? [x.turn] : []));

export function layout(ms: Message[], live: boolean): (Message | Worked | Changes)[] {
  const out: (Message | Worked | Changes)[] = [];
  // A question is one more step: the composer is where it is answered. A
  // rendered HTML page is part of the answer and stays out of the fold.
  const isWork = (m: Message) => m.role === "reasoning" || (m.role === "tool" && !htmlRef(m)) || m.role === "question" || m.role === "assistant";
  let start = 0;
  const flush = (end: number, last: boolean) => {
    // The turn's git diff is only read by the box of its changes.
    const seg = ms.slice(start, end).filter((m) => m.role !== "changes");
    if (!seg.length && !(last && live)) return;
    let head: Message | undefined;
    if (seg[0]?.role === "user") { head = seg[0]; out.push(seg.shift()!); }
    if (last && live) {
      // The text the model is writing last reads as the answer, as in Claude.
      const tail = seg.at(-1)?.role === "assistant" ? seg.at(-1) : undefined;
      out.push({ worked: true, live: true, id: `working:${head?.id ?? seg[0]?.id ?? "turn"}`, items: seg.filter((m) => isWork(m) && m !== tail), secs: 0, since: head?.created_at || seg[0]?.created_at });
      out.push(...seg.filter((m) => !isWork(m) || m === tail));
      return;
    }
    // A finished turn that edited files ends with the box of its changes.
    const files = turnFiles(ms.slice(start, end));
    const box = () => { if (files.length) { const id = head?.id ?? seg[0].id; out.push({ changes: true, id: `changes:${id}`, turn: { id, files } }); } };
    let answer = -1;
    for (let i = seg.length - 1; i >= 0; i--) if (seg[i].role === "assistant") { answer = i; break; }
    const work = seg.filter((m, i) => isWork(m) && i !== answer);
    if (!work.length) { out.push(...seg); box(); return; }
    const from = Date.parse(head?.created_at || work[0].created_at);
    const to = Date.parse((answer >= 0 ? seg[answer] : work[work.length - 1]).created_at);
    out.push({ worked: true, id: `worked:${work[0].id}`, items: work, secs: Number.isFinite(from) && Number.isFinite(to) ? (to - from) / 1000 : 0 });
    out.push(...seg.filter((m, i) => !(isWork(m) && i !== answer)));
    box();
  };
  let run = "";
  ms.forEach((m, i) => {
    if (i > start && (m.role === "user" || m.role === "model-change" || m.role === "clear" || (m.run_id && run && m.run_id !== run))) { flush(i, false); start = i; run = ""; }
    if (m.run_id) run = m.run_id;
  });
  flush(ms.length, true);
  return out;
}

// elapsed writes a duration the short way: "45s", "1m 59s", "1h 3m".
const elapsed = (secs: number) => {
  const s = Math.max(0, Math.round(secs));
  if (s < 60) return `${s}s`;
  if (s < 3600) return `${Math.floor(s / 60)}m ${s % 60}s`;
  return `${Math.floor(s / 3600)}h ${Math.floor((s % 3600) / 60)}m`;
};

// WorkedBlock is a turn's work: while it runs, a ticking "Working for 12s"
// line and its steps; once it ends, a folded "Worked for 1m 59s".
export function WorkedBlock({ Entry, items, secs, live = false, since }: { Entry: React.ComponentType<{ m: Message }>; items: Message[]; secs: number; live?: boolean; since?: string }) {
  const [open, setOpen] = useState(false);
  const [now, setNow] = useState(() => Date.now());
  useEffect(() => {
    if (!live) return;
    const id = setInterval(() => setNow(Date.now()), 1000);
    return () => clearInterval(id);
  }, [live]);
  const from = Date.parse(since ?? "");
  if (live) {
    return (
      <div className="space-y-1.5">
        <div role="status" className="flex items-center gap-3 text-xs text-slate-500 dark:text-muted">
          <span className="shrink-0 font-medium tabular-nums">Working for {Number.isFinite(from) ? elapsed((now - from) / 1000) : "…"}</span>
          <span className="h-px flex-1 bg-slate-200 dark:bg-border" />
        </div>
        {/* While the turn runs, what the agent writes between steps reads as a
            normal answer; once it ends it folds into "Worked for…" with the rest. */}
        {stepRuns(items).map((r) => stepKind(r[0]) === "note" && !r[0].tool
          ? <div key={r[0].id} className="space-y-4">{r.map((m) => <Entry key={m.id} m={m} />)}</div>
          : <StepLine key={r[0].id} run={r} />)}
      </div>
    );
  }
  return (
    <div className="text-sm">
      <button type="button" aria-expanded={open} onClick={() => setOpen(!open)}
        className="flex w-full items-center gap-3 py-0.5 text-xs text-slate-500 hover:text-slate-900 dark:text-muted dark:hover:text-white">
        <span className="shrink-0 font-medium tabular-nums">{secs >= 1 ? `Worked for ${elapsed(secs)}` : "Worked"}</span>
        <span className="h-px flex-1 bg-slate-200 dark:bg-border" />
      </button>
      {open && items.length > 0 && <div className="mt-1.5 space-y-1.5">{stepRuns(items).map((r) => <StepLine key={r[0].id} run={r} />)}</div>}
    </div>
  );
}

// Reasoning shows the model's thinking folded to its title; open while it thinks.
export function Reasoning({ m }: { m: Message }) {
  const [opened, setOpen] = useState<boolean | null>(null);
  const open = opened ?? m.streaming;
  if (!m.text.trim() && !m.streaming) return null;
  return (
    <div className="text-sm">
      <button type="button" aria-expanded={open} onClick={() => setOpen(!open)}
        className="flex items-center gap-2 py-0.5 text-xs font-medium text-slate-500 hover:text-slate-900 dark:text-muted dark:hover:text-white">
        <Brain className="size-3.5 shrink-0" />{m.streaming ? "Pensando…" : "Razonamiento"}
        {open ? <CaretDown weight="regular" className="size-3 shrink-0" /> : <CaretRight weight="regular" className="size-3 shrink-0" />}
      </button>
      {open && m.text.trim() && (
        <div className="mt-0.5 border-l-2 border-slate-200 pl-3 text-[0.8125rem] leading-relaxed text-slate-500 dark:border-border dark:text-muted [&_*]:!text-inherit"><Md>{m.text}</Md></div>
      )}
    </div>
  );
}

