import { useState, type ReactNode } from "react";
import { CaretDown, CaretRight, CheckCircle, Circle, CircleNotch, FileText, Globe, MagnifyingGlass, PencilSimple, Robot, TerminalWindow, Wrench, XCircle } from "@phosphor-icons/react";
import type { Message, Question, ToolCall } from "../api/client";

// The cards of the tool calls in the chat, one per Dory tool kind. Every
// harness maps its tools onto these kinds in the runtime (ToolCall), so a
// Claude Edit and a Codex apply_patch draw the same card.

const done = ["completed", "failed", "error", "declined", "interrupted", "cancelled"];
export const isLive = (m: Message) => m.streaming || !done.includes(m.status ?? "");
export const isFailed = (m: Message) => m.status === "failed" || m.status === "error";
const base = (p = "") => p.replace(/\/+$/, "").split("/").pop() || p;
const mono = "font-mono text-[0.75rem]";

function State({ m }: { m: Message }) {
  if (isLive(m)) return <CircleNotch weight="regular" className="size-3.5 shrink-0 animate-spin text-slate-400" aria-label="En curso" />;
  if (isFailed(m)) return <XCircle weight="fill" className="size-3.5 shrink-0 text-red-500" aria-label="Falló" />;
  return null;
}

// Row is the one-line head every card shares; a click unfolds its body.
function Row({ icon, children, body: given, m, open: initial = false }: { icon: ReactNode; children: ReactNode; body?: ReactNode; m: Message; open?: boolean }) {
  const [open, setOpen] = useState(initial);
  // A failed call always unfolds to its error, whatever its card shows.
  const error = isFailed(m) && m.text.trim() ? <pre className={`${mono} max-h-48 overflow-auto whitespace-pre-wrap break-all rounded-lg bg-red-50 p-2.5 text-red-700 dark:bg-red-950/40 dark:text-red-300`}>{m.text}</pre> : null;
  const body = error && !given ? error : given;
  return (
    <div className="text-[0.8125rem] leading-relaxed text-slate-500 dark:text-muted">
      <button type="button" disabled={!body} onClick={() => setOpen(!open)} aria-expanded={body ? open : undefined}
        className="flex w-full min-w-0 items-center gap-2 text-left enabled:hover:text-slate-900 dark:enabled:hover:text-white">
        <span className="shrink-0">{icon}</span>
        <span className={`flex min-w-0 flex-1 items-center gap-1.5 ${isFailed(m) ? "text-red-600 dark:text-red-400" : ""}`}>{children}</span>
        <State m={m} />
        {body && (open ? <CaretDown className="size-3 shrink-0" /> : <CaretRight className="size-3 shrink-0" />)}
      </button>
      {open && body && <div className="mb-1 ml-6 mt-1">{body}</div>}
    </div>
  );
}

const Output = ({ text }: { text: string }) => text.trim()
  ? <pre className={`${mono} max-h-72 overflow-auto whitespace-pre-wrap break-all rounded-lg bg-slate-100 p-2.5 text-slate-700 dark:bg-raised dark:text-text-secondary`}>{text}</pre>
  : null;

const Count = ({ added, removed }: { added: number; removed: number }) => (added || removed)
  ? <span className={`${mono} shrink-0 tabular-nums`}><span className="text-emerald-600 dark:text-emerald-400">+{added}</span> <span className="text-red-600 dark:text-red-400">-{removed}</span></span>
  : null;

// EditCard is one compact line per edit; the turn's box at its end shows the changes.
function EditCard({ m, t }: { m: Message; t: ToolCall }) {
  const files = t.files ?? [];
  if (files.length !== 1) {
    return (
      <Row m={m} icon={<PencilSimple className="size-4" />}>
        <span className="truncate">{isLive(m) ? "Editando" : "Editó"} {files.length} ficheros</span>
        <Count added={files.reduce((n, f) => n + f.added, 0)} removed={files.reduce((n, f) => n + f.removed, 0)} />
      </Row>
    );
  }
  const f = files[0];
  const verb = f.change === "add" ? (isLive(m) ? "Creando" : "Creó") : f.change === "delete" ? "Borró" : isLive(m) ? "Editando" : "Editó";
  return (
    <Row m={m} icon={<PencilSimple className="size-4" />}>
      <span className="shrink-0">{verb}</span><span title={f.path} className={`${mono} truncate text-slate-700 dark:text-text-secondary`}>{base(f.path)}</span><Count added={f.added} removed={f.removed} />
    </Row>
  );
}

function CommandCard({ m, t }: { m: Message; t: ToolCall }) {
  const code = t.exit_code;
  return (
    <Row m={m} icon={<TerminalWindow className="size-4" />}
      body={<div className="space-y-1.5"><Output text={`$ ${t.command ?? ""}`} />{m.text && <Output text={m.text} />}{code != null && code !== 0 && <p className="text-xs text-red-600 dark:text-red-400">Código de salida {code}</p>}</div>}>
      {/* What the agent says the command is for reads first; the command follows. */}
      {t.summary && <span className="min-w-0 shrink truncate text-slate-700 dark:text-text-secondary">{t.summary}</span>}
      <span className={`${mono} min-w-0 truncate ${t.summary ? "flex-1 opacity-70" : "text-slate-700 dark:text-text-secondary"}`}>{t.command || m.detail || "comando"}</span>
      {code != null && code !== 0 && <span className={`${mono} shrink-0 text-red-600 dark:text-red-400`}>exit {code}</span>}
    </Row>
  );
}

// TodoCard is the agent's task list; it updates in place as steps finish.
export function TodoCard({ m }: { m: Message }) {
  const todos = m.tool?.todos ?? [];
  const finished = todos.filter((x) => x.status === "completed").length;
  return (
    <div className="rounded-xl border border-slate-200 bg-white px-3.5 py-2.5 dark:border-border dark:bg-surface">
      <div className="mb-1.5 flex items-center gap-2 text-xs font-medium text-slate-500 dark:text-muted">Tareas<span className="tabular-nums">{finished}/{todos.length}</span></div>
      <ul className="space-y-1">
        {todos.map((x, i) => (
          <li key={i} className="flex items-start gap-2 text-[0.8125rem]">
            {x.status === "completed" ? <CheckCircle weight="fill" className="mt-0.5 size-4 shrink-0 text-emerald-600 dark:text-emerald-400" />
              : x.status === "in_progress" ? <CircleNotch weight="regular" className="mt-0.5 size-4 shrink-0 animate-spin text-blue-500" />
              : <Circle className="mt-0.5 size-4 shrink-0 text-slate-300 dark:text-border-strong" />}
            <span className={x.status === "completed" ? "text-slate-400 line-through" : x.status === "in_progress" ? "font-medium text-slate-900 dark:text-white" : "text-slate-600 dark:text-text-secondary"}>{x.status === "in_progress" && x.active ? x.active : x.text}</span>
          </li>
        ))}
      </ul>
    </div>
  );
}

// results counts what a run of searches found: one line per match or file.
const results = (run: Message[]) => run.reduce((n, x) => n + x.text.split("\n").filter((l) => l.trim() && !/^(No files found|No matches found|Found \d+)/.test(l)).length, 0);

// GroupCard folds consecutive reads or searches into one line ("Leyó 5 ficheros").
export function GroupCard({ run }: { run: Message[] }) {
  const m = run[run.length - 1];
  const t = m.tool!;
  const read = t.kind === "read";
  const label = (x: Message) => read ? `${x.tool?.path ?? ""}${x.tool?.lines ? `:${x.tool.lines}` : ""}` : `${x.tool?.pattern ?? ""}${x.tool?.path ? `  en ${x.tool.path}` : ""}`;
  const head = run.length > 1
    ? `${read ? (isLive(m) ? "Leyendo" : "Leyó") : isLive(m) ? "Buscando" : "Buscó"} ${run.length} ${read ? "ficheros" : "patrones"}`
    : read ? `${isLive(m) ? "Leyendo" : "Leyó"} ` : `${isLive(m) ? "Buscando" : "Buscó"} `;
  return (
    <Row m={m} icon={read ? <FileText className="size-4" /> : <MagnifyingGlass weight="regular" className="size-4" />}
      body={<ul className="space-y-0.5">{run.map((x) => <li key={x.id} className={`${mono} truncate`} title={label(x)}>{label(x)}</li>)}</ul>}>
      <span className="shrink-0">{head}</span>{run.length === 1 && <span className={`${mono} truncate text-slate-700 dark:text-text-secondary`}>{read ? base(t.path) : t.pattern}</span>}
      {!read && !isLive(m) && !isFailed(m) && <span className="shrink-0 text-[0.6875rem] tabular-nums opacity-70">{results(run)} resultados</span>}
    </Row>
  );
}

function WebCard({ m, t }: { m: Message; t: ToolCall }) {
  let host = "";
  try { host = t.url ? new URL(t.url).hostname : ""; } catch { /* not a URL */ }
  return (
    <Row m={m} icon={<Globe className="size-4" />}
      body={m.text ? <Output text={m.text} /> : undefined}>
      {t.url ? <><span className="shrink-0">Leyó</span><a href={t.url} target="_blank" rel="noreferrer" onClick={(e) => e.stopPropagation()} className="truncate underline decoration-dotted underline-offset-2">{host || t.url}</a></>
        : <><span className="shrink-0">Buscó en la web</span><span className="truncate text-slate-700 dark:text-text-secondary">«{t.query}»</span></>}
    </Row>
  );
}

function AgentCard({ m, t }: { m: Message; t: ToolCall }) {
  return (
    <div className="rounded-xl border border-slate-200 px-3 py-2 dark:border-border">
      <Row m={m} icon={<Robot className="size-4" />} body={m.text ? <Output text={m.text} /> : undefined}>
        <span className="shrink-0 font-medium text-slate-700 dark:text-text-secondary">{t.agent || "Subagente"}</span><span className="min-w-0 flex-1 truncate">{t.summary}</span>
        {!!t.steps && <span className="shrink-0 text-[0.6875rem] tabular-nums opacity-70">{t.steps} {t.steps === 1 ? "herramienta" : "herramientas"}</span>}
      </Row>
    </div>
  );
}

function McpCard({ m, t }: { m: Message; t: ToolCall }) {
  const chip = "rounded-md bg-slate-100 px-1.5 py-0.5 font-mono text-[0.6875rem] text-slate-600 dark:bg-raised dark:text-text-secondary";
  return (
    <Row m={m} icon={<Wrench className="size-4" />}
      body={<div className="space-y-1.5">{t.input && <Output text={pretty(t.input)} />}{m.text && <Output text={m.text} />}</div>}>
      {t.kind === "mcp" ? <><span className={chip}>{t.server}</span><span className={`${chip} truncate`}>{t.tool}</span></> : <span className="truncate">{t.name}</span>}
    </Row>
  );
}

const pretty = (s: string) => { try { return JSON.stringify(JSON.parse(s), null, 2); } catch { return s; } };

// isAsk is a call to Dory's ask_user (Claude names it mcp__dory__ask_user,
// OpenCode dory_ask_user): the chat shows it as a question card.
export const isAsk = (m: Message) => m.role === "tool" && !!m.tool && (m.tool.tool === "ask_user" || /(^|_)ask_user$/.test(m.tool.name));

// askQuestions reads the questions of an ask_user call from its arguments.
export function askQuestions(m: Message): Question[] {
  try {
    const a = JSON.parse(m.tool?.input ?? "{}");
    const qs: Question[] = (a.questions ?? []).map((q: Question & { multiSelect?: boolean }) => ({ ...q, multi_select: q.multi_select ?? q.multiSelect }));
    // A lone "question" repeated in "questions" keeps the richer entry (header, options).
    return a.question && !qs.some((q) => q.question === a.question) ? [{ question: a.question }, ...qs] : qs;
  } catch { return []; }
}

// ToolCard draws one tool call by its kind.
export function ToolCard({ m }: { m: Message }) {
  const t = m.tool;
  // The call to Dory's own ask_user shows as its question card instead.
  if (!t || isAsk(m)) return null;
  switch (t.kind) {
    case "edit": return <EditCard m={m} t={t} />;
    case "command": return <CommandCard m={m} t={t} />;
    case "todo": return <TodoCard m={m} />;
    case "plan": case "question": return null; // shown by their own message roles
    case "read": case "search": return <GroupCard run={[m]} />;
    case "web": return <WebCard m={m} t={t} />;
    case "agent": return <AgentCard m={m} t={t} />;
    default: return <McpCard m={m} t={t} />;
  }
}
