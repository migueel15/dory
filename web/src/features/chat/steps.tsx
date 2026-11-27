import { Brain,CaretDown,CaretRight,ChatText,FileText,Globe,MagnifyingGlass,PencilSimple,Question as QuestionIcon,Robot,TerminalWindow,Wrench,XCircle } from "@phosphor-icons/react";
import { useEffect,useRef,useState } from "react";
import { type Message } from "../../api/client";
import { Md } from "../../components/markdown";
import { t } from "../../i18n/es";
import { htmlRef } from "./html-card";
import { NativeQuestionCard,QuestionCard } from "./question-card";
import { GroupCard,TodoCard,ToolCard,askQuestions,isAsk } from "./tool-card";
import { turnFiles,type Turn } from "./turn-changes";

const shortTime = new Intl.DateTimeFormat("es-ES", { hour: "2-digit", minute: "2-digit" });

function formatTime(iso?: string): string {
  if (!iso) return "";
  const d = new Date(iso);
  return Number.isNaN(d.getTime()) ? "" : shortTime.format(d);
}

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
    if (m.role === "reasoning" && !m.text.trim()) continue;
    const last = out.at(-1);
    if (last && !isQuestion(m) && !isQuestion(last[0]) && stepKind(last[0]) === stepKind(m)) last.push(m);
    else out.push([m]);
  }
  return out;
}

export const isQuestion = (m: Message) => isAsk(m) || m.tool?.kind === "question" || m.role === "question";

export const oneLine = (text = "") => text.replace(/[`*_#>]/g, "").replace(/\s+/g, " ").trim();

// thoughtPreview extracts the latest bold title (**...**) if present, otherwise
// the first sentence of a reasoning block, stripped of Markdown.
export function thoughtPreview(text = ""): string {
  const trimmed = text.trim();
  if (!trimmed) return "";
  const bolds = [...trimmed.matchAll(/\*\*([^*]+)\*\*/g)];
  const lastBold = bolds.at(-1)?.[1]?.trim();
  if (lastBold) return oneLine(lastBold);
  const firstLine = trimmed.split(/\n+/)[0] ?? "";
  const sentence = firstLine.match(/^(.+?[.!?])(?:\s|$)/)?.[1] ?? firstLine;
  return oneLine(sentence);
}

// settleSupersededReasoning marks any reasoning item followed by another item
// as finished even if the provider never closed its streaming flag.
function settleSupersededReasoning(items: Message[]): Message[] {
  return items.map((m, i) =>
    m.role === "reasoning" && m.streaming && i < items.length - 1 ? { ...m, streaming: false } : m,
  );
}

// QuestionStep is a standalone row for an agent question: shows the question
// and a summary of the answer ("¿Uso JWT? → JWT"), expanding on click.
export function QuestionStep({ m }: { m: Message }) {
  const [open, setOpen] = useState(false);
  const qs = isAsk(m) ? askQuestions(m) : m.tool?.questions ?? [];
  const first = qs[0]?.question ?? m.text;
  const answers = qs.map((q) => q.answer?.join(", ")).filter(Boolean);
  const answered = answers.length > 0;
  return (
    <div className="text-[0.8125rem] leading-relaxed text-slate-500 dark:text-muted">
      <button type="button" onClick={() => setOpen(!open)} aria-expanded={open}
        className="flex min-h-6 w-full min-w-0 items-center gap-2 text-left hover:text-slate-900 dark:hover:text-white">
        <QuestionIcon weight="regular" className="size-4 shrink-0" />
        <span className="min-w-0 flex-1 truncate">
          {oneLine(first) || t("question.title")}
          {qs.length > 1 ? ` (+${qs.length - 1})` : ""}
          {answered && <span className="text-slate-700 dark:text-text-secondary"> → {oneLine(answers[0])}</span>}
        </span>
        {open ? <CaretDown weight="regular" className="size-3 shrink-0 opacity-70" /> : <CaretRight weight="regular" className="size-3 shrink-0 opacity-70" />}
      </button>
      {open && <div className="mb-1 ml-6 mt-1.5">{isAsk(m) || m.role === "question" ? <QuestionCard m={m} /> : <NativeQuestionCard m={m} />}</div>}
    </div>
  );
}

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
  const line = kind === "think" ? (thoughtPreview(m.text) || (live ? "Pensando" : "Pensó"))
    : kind === "note" ? oneLine(m.text)
    : kind === "cmd" ? `${live ? "Ejecutando" : bad ? "Falló" : "Ejecutó"} ${commandName(m.detail) || "comando"}`
    : m.title || "Herramienta";
  const full = (x: Message) => x.role === "tool"
    ? <pre key={x.id} className="max-h-60 overflow-auto whitespace-pre-wrap break-all rounded bg-slate-100 p-2 font-mono text-[0.6875rem] dark:bg-raised">{[x.detail, x.text].filter(Boolean).join("\n\n")}</pre>
    : <div key={x.id} className="[&_*]:!text-inherit [&_p]:my-0.5"><Md>{x.text}</Md></div>;
  return (
    <div className={`text-[0.8125rem] leading-relaxed ${kind === "note" ? "text-slate-700 dark:text-text-secondary" : "text-slate-500 dark:text-muted"}`}>
      <button type="button" onClick={() => setOpen(!open)} aria-expanded={open}
        className="flex min-h-6 w-full min-w-0 items-center gap-2 text-left hover:text-slate-900 dark:hover:text-white">
        {bad ? <XCircle weight="regular" className="size-4 shrink-0 text-red-500" /> : icon}
        <span className={`min-w-0 flex-1 truncate ${live ? "shimmer-text" : ""} ${bad ? "text-red-600 dark:text-red-400" : ""}`}>{line}</span>
        {run.length > 1 && <span className="shrink-0 text-[0.6875rem] tabular-nums opacity-60">{run.length}</span>}
      </button>
      {open && <div className="mb-1 ml-6 mt-1 space-y-1.5">{run.map(full)}</div>}
    </div>
  );
}

// Worked is a turn's work folded into one entry.
export type Worked = {
  worked: true;
  id: string;
  items: Message[];
  secs: number;
  live?: boolean;
  since?: string;
  endedAt?: string;
  stopped?: boolean;
  expanded?: boolean;
};

// layout lays a conversation out turn by turn. In a finished turn, everything
// between the user's message and the final answer folds into one "Trabajó
// durante…" entry (leaving questions, errors and HTML cards outside the fold);
// the turn still running shows its live status and latest group.
export type Changes = { changes: true; id: string; turn: Turn };

// changeTurns lists the turns that edited files, oldest first.
export const changeTurns = (ms: Message[]): Turn[] => layout(ms, false).flatMap((x) => ("changes" in x ? [x.turn] : []));

export function layout(ms: Message[], live: boolean): (Message | Worked | Changes)[] {
  const out: (Message | Worked | Changes)[] = [];
  // Questions, rendered HTML pages and errors stay outside the finished fold.
  const isWork = (m: Message) =>
    m.role === "reasoning" ||
    (m.role === "tool" && !htmlRef(m) && !isQuestion(m)) ||
    m.role === "assistant";
  const isLiveWork = (m: Message) => isWork(m) || isQuestion(m);
  let start = 0;
  const flush = (end: number, last: boolean) => {
    // The turn's git diff is only read by the box of its changes.
    const seg = ms.slice(start, end).filter((m) => m.role !== "changes");
    if (!seg.length && !(last && live)) return;
    let head: Message | undefined;
    if (seg[0]?.role === "user") { head = seg[0]; out.push(seg.shift()!); }
    if (last && live) {
      // The text the model is writing last reads as the answer.
      const tail = seg.at(-1)?.role === "assistant" ? seg.at(-1) : undefined;
      out.push({
        worked: true,
        live: true,
        id: `working:${head?.id ?? seg[0]?.id ?? "turn"}`,
        items: seg.filter((m) => isLiveWork(m) && m !== tail),
        secs: 0,
        since: head?.created_at || seg[0]?.created_at,
      });
      out.push(...seg.filter((m) => !isLiveWork(m) || m === tail));
      return;
    }
    // A finished turn that edited files ends with the box of its changes.
    const files = turnFiles(ms.slice(start, end));
    const box = () => { if (files.length) { const id = head?.id ?? seg[0].id; out.push({ changes: true, id: `changes:${id}`, turn: { id, files } }); } };
    let answer = -1;
    for (let i = seg.length - 1; i >= 0; i--) if (seg[i].role === "assistant") { answer = i; break; }
    const work = seg.filter((m, i) => isWork(m) && i !== answer);
    if (!work.length) { out.push(...seg); box(); return; }
    const stopped = seg.some((m) => m.role === "interrupt");
    const turnFailed = seg.some((m) => m.status === "failed");
    const from = Date.parse(head?.created_at || work[0].created_at);
    const endedAt = (answer >= 0 ? seg[answer] : work[work.length - 1]).created_at;
    const to = Date.parse(endedAt);
    out.push({
      worked: true,
      id: `worked:${work[0].id}`,
      items: work,
      secs: Number.isFinite(from) && Number.isFinite(to) ? (to - from) / 1000 : 0,
      endedAt,
      stopped,
      expanded: turnFailed,
    });
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

const baseName = (p = "") => p.replace(/\/+$/, "").split("/").pop() || p;

function actionIcon(m: Message) {
  const kind = m.tool?.kind;
  if (kind === "read") return <FileText className="size-4 shrink-0" />;
  if (kind === "search") return <MagnifyingGlass weight="regular" className="size-4 shrink-0" />;
  if (kind === "edit") return <PencilSimple className="size-4 shrink-0" />;
  if (kind === "command" || shells.has(m.title ?? "")) return <TerminalWindow weight="regular" className="size-4 shrink-0" />;
  if (kind === "web") return <Globe className="size-4 shrink-0" />;
  if (kind === "agent") return <Robot className="size-4 shrink-0" />;
  return <Wrench weight="regular" className="size-4 shrink-0" />;
}

function actionLabel(m: Message): string {
  const live = running(m);
  const bad = failed(m);
  const tc = m.tool;
  if (tc) {
    switch (tc.kind) {
      case "read":
        return `${live ? "Leyendo" : "Leyó"} ${baseName(tc.path) || "fichero"}`;
      case "search":
        return `${live ? "Buscando" : "Buscó"} ${tc.pattern || "en el código"}`;
      case "edit": {
        const files = tc.files ?? [];
        if (files.length === 1) {
          const f = files[0];
          const verb = f.change === "add" ? (live ? "Creando" : "Creó") : f.change === "delete" ? "Borró" : live ? "Editando" : "Editó";
          return `${verb} ${baseName(f.path)}`;
        }
        return `${live ? "Editando" : "Editó"} ${files.length} ficheros`;
      }
      case "command":
        return tc.summary || `${live ? "Ejecutando" : bad ? "Falló" : "Ejecutó"} ${commandName(tc.command || m.detail) || "comando"}`;
      case "web":
        return tc.url ? `${live ? "Leyendo" : "Leyó"} ${tc.url}` : `${live ? "Buscando" : "Buscó"} «${tc.query ?? ""}»`;
      case "agent":
        return `${tc.agent || "Subagente"}${tc.summary ? `: ${tc.summary}` : ""}`;
      case "mcp":
        return tc.tool ? `${tc.server ?? "MCP"} · ${tc.tool}` : tc.name || m.title || "Herramienta";
      default:
        return tc.name || m.title || "Herramienta";
    }
  }
  if (shells.has(m.title ?? "")) {
    return `${live ? "Ejecutando" : bad ? "Falló" : "Ejecutó"} ${commandName(m.detail) || "comando"}`;
  }
  return m.title || "Herramienta";
}

export type WorkBlock =
  | { kind: "reasoning"; items: Message[] }
  | { kind: "note"; m: Message }
  | { kind: "question"; m: Message }
  | { kind: "todo"; m: Message }
  | { kind: "actions"; items: Message[] };

// workBlocks splits a turn's work at every transition between reasoning and
// actions (tool calls); assistant notes, questions and todo lists stay as
// standalone blocks.
export function workBlocks(rawItems: Message[]): WorkBlock[] {
  const items = settleSupersededReasoning(rawItems);
  const out: WorkBlock[] = [];
  for (const m of items) {
    if (m.role === "reasoning") {
      if (!m.text.trim()) continue;
      const last = out.at(-1);
      if (last?.kind === "reasoning") last.items.push(m);
      else out.push({ kind: "reasoning", items: [m] });
    } else if (m.role === "assistant") {
      out.push({ kind: "note", m });
    } else if (isQuestion(m)) {
      out.push({ kind: "question", m });
    } else if (m.tool?.kind === "todo") {
      out.push({ kind: "todo", m });
    } else {
      const last = out.at(-1);
      if (last?.kind === "actions") last.items.push(m);
      else out.push({ kind: "actions", items: [m] });
    }
  }
  return out;
}

function actionCategory(m: Message): "read" | "search" | "edit" | "command" | "web" | "agent" | "other" {
  const kind = m.tool?.kind;
  if (kind === "read" || kind === "search" || kind === "edit" || kind === "command" || kind === "web" || kind === "agent") return kind;
  if (shells.has(m.title ?? "")) return "command";
  return "other";
}

function groupSummary(tools: Message[]): string {
  const counts = new Map<ReturnType<typeof actionCategory>, number>();
  for (const m of tools) {
    const c = actionCategory(m);
    counts.set(c, (counts.get(c) ?? 0) + 1);
  }
  if (counts.size === 1) {
    const [[cat, n]] = counts.entries();
    switch (cat) {
      case "read": return n === 1 ? "Leyó 1 fichero" : `Leyó ${n} ficheros`;
      case "search": return n === 1 ? "Hizo 1 búsqueda" : `Hizo ${n} búsquedas`;
      case "edit": return n === 1 ? "Editó 1 fichero" : `Editó ${n} ficheros`;
      case "command": return n === 1 ? "Ejecutó 1 comando" : `Ejecutó ${n} comandos`;
      case "web": return n === 1 ? "Consultó 1 página" : `Consultó ${n} páginas`;
      case "agent": return n === 1 ? "Lanzó 1 subagente" : `Lanzó ${n} subagentes`;
      default: return n === 1 ? "Usó 1 herramienta" : `Usó ${n} herramientas`;
    }
  }
  const labels: Record<ReturnType<typeof actionCategory>, [string, string]> = {
    read: ["lectura", "lecturas"],
    search: ["búsqueda", "búsquedas"],
    edit: ["edición", "ediciones"],
    command: ["comando", "comandos"],
    web: ["consulta web", "consultas web"],
    agent: ["subagente", "subagentes"],
    other: ["herramienta", "herramientas"],
  };
  return Array.from(counts.entries())
    .map(([cat, n]) => `${n} ${n === 1 ? labels[cat][0] : labels[cat][1]}`)
    .join(", ");
}

// ReasoningScroll shows full reasoning text in a bounded scrollable box with
// top/bottom fade masks that sticks to the bottom only if already at the bottom.
function ReasoningScroll({ items }: { items: Message[] }) {
  const box = useRef<HTMLDivElement>(null);
  const stick = useRef(true);
  const lastText = items.map((m) => m.text).join("");
  useEffect(() => {
    const el = box.current;
    if (el && stick.current) el.scrollTop = el.scrollHeight;
  }, [lastText]);
  const onScroll = () => {
    const el = box.current;
    if (!el) return;
    stick.current = el.scrollHeight - el.scrollTop - el.clientHeight < 24;
  };
  return (
    <div
      ref={box}
      onScroll={onScroll}
      style={{ maskImage: "linear-gradient(to bottom, transparent, black 12px, black calc(100% - 12px), transparent)" }}
      className="mb-1 ml-6 mt-1 max-h-[min(18rem,50vh)] space-y-2 overflow-y-auto py-2 text-[0.8125rem] leading-relaxed text-slate-500 dark:text-muted [&_*]:!text-inherit [&_p]:my-0.5"
    >
      {items.map((m) => <div key={m.id}><Md>{m.text}</Md></div>)}
    </div>
  );
}

// ReasoningGroup renders a block of consecutive reasoning items as a single
// preview line. Clicking it toggles only the full reasoning text.
function ReasoningGroup({ items, isLive }: { items: Message[]; isLive: boolean }) {
  const [open, setOpen] = useState(false);
  const lastThought = items.at(-1);
  const label = (lastThought && thoughtPreview(lastThought.text)) || (isLive ? "Pensando" : "Pensó");
  const time = formatTime(lastThought?.created_at);

  return (
    <div className="text-[0.8125rem] leading-relaxed text-slate-500 dark:text-muted">
      <button type="button" onClick={() => setOpen(!open)} aria-expanded={open}
        className="flex min-h-6 w-full min-w-0 items-center gap-2 text-left hover:text-slate-900 dark:hover:text-white">
        <Brain weight="regular" className="size-4 shrink-0" />
        <span className={`min-w-0 flex-1 truncate ${isLive ? "shimmer-text" : ""}`}>{label}</span>
        {!isLive && time && <span className="shrink-0 text-[0.6875rem] tabular-nums opacity-60">{time}</span>}
        {open ? <CaretDown weight="regular" className="size-3 shrink-0 opacity-70" /> : <CaretRight weight="regular" className="size-3 shrink-0 opacity-70" />}
      </button>
      {open && <ReasoningScroll items={items} />}
    </div>
  );
}

// ActionGroup renders a block of consecutive tool calls. While any tool in the
// group is running it shows the latest action with a shimmer; once settled it
// collapses to a summary line. Clicking toggles the actions in this group.
function ActionGroup({ items, isLive }: { items: Message[]; isLive: boolean }) {
  const [open, setOpen] = useState(false);
  const lastTool = items[items.length - 1];
  const active = isLive && items.some(running);
  const hasError = items.some(failed);
  const time = formatTime(lastTool?.created_at);

  const label = active
    ? actionLabel(lastTool)
    : items.length === 1
      ? actionLabel(items[0])
      : groupSummary(items);
  const sameCategory = items.every((m) => actionCategory(m) === actionCategory(items[0]));
  const icon = active
    ? (failed(lastTool) ? <XCircle weight="regular" className="size-4 shrink-0 text-red-500" /> : actionIcon(lastTool))
    : hasError
      ? <XCircle weight="regular" className="size-4 shrink-0 text-red-500" />
      : items.length === 1 || sameCategory
        ? actionIcon(items[0])
        : <Wrench weight="regular" className="size-4 shrink-0" />;

  return (
    <div className="text-[0.8125rem] leading-relaxed text-slate-500 dark:text-muted">
      <button type="button" onClick={() => setOpen(!open)} aria-expanded={open}
        className="flex min-h-6 w-full min-w-0 items-center gap-2 text-left hover:text-slate-900 dark:hover:text-white">
        {icon}
        <span className={`min-w-0 flex-1 truncate ${active ? "shimmer-text" : ""} ${!active && hasError ? "text-red-600 dark:text-red-400" : ""}`}>{label}</span>
        {active && items.length > 1 && (
          <span className="shrink-0 text-[0.6875rem] tabular-nums opacity-60">{items.length}</span>
        )}
        {!active && time && <span className="shrink-0 text-[0.6875rem] tabular-nums opacity-60">{time}</span>}
        {open ? <CaretDown weight="regular" className="size-3 shrink-0 opacity-70" /> : <CaretRight weight="regular" className="size-3 shrink-0 opacity-70" />}
      </button>
      {open && (
        <div className="mb-1 ml-6 mt-1 space-y-1 border-l border-slate-200 pl-3 dark:border-border">
          {stepRuns(items).map((r) => <StepLine key={r[0].id} run={r} />)}
        </div>
      )}
    </div>
  );
}

function NoteText({ m }: { m: Message }) {
  if (!m.text.trim()) return null;
  return (
    <div className="text-[0.875rem] leading-relaxed text-slate-900 dark:text-white [&_*]:!text-inherit [&_p]:my-0.5">
      <Md>{m.text}</Md>
    </div>
  );
}

// WorkedBlock is a turn's work: while it runs, a ticking "Trabajando durante 12s"
// header with a live two-line group; once it ends, a folded "Trabajó durante 1m 59s".
export function WorkedBlock({
  Entry: _Entry,
  items,
  secs,
  live = false,
  since,
  endedAt,
  stopped = false,
  expanded = false,
}: {
  Entry: React.ComponentType<{ m: Message }>;
  items: Message[];
  secs: number;
  live?: boolean;
  since?: string;
  endedAt?: string;
  stopped?: boolean;
  expanded?: boolean;
}) {
  const [open, setOpen] = useState(expanded);
  const [now, setNow] = useState(() => Date.now());
  useEffect(() => {
    if (!live) return;
    const id = setInterval(() => setNow(Date.now()), 1000);
    return () => clearInterval(id);
  }, [live]);
  const from = Date.parse(since ?? "");
  const blocks = workBlocks(items);
  const renderBlock = (b: WorkBlock, index: number) => {
    const isLastLive = live && index === blocks.length - 1;
    switch (b.kind) {
      case "reasoning":
        return <ReasoningGroup key={b.items[0].id} items={b.items} isLive={isLastLive} />;
      case "note":
        return <NoteText key={b.m.id} m={b.m} />;
      case "question":
        return <QuestionStep key={b.m.id} m={b.m} />;
      case "todo":
        return <TodoCard key={b.m.id} m={b.m} />;
      case "actions":
        return <ActionGroup key={b.items[0].id} items={b.items} isLive={isLastLive} />;
    }
  };
  if (live) {
    const lastBlock = blocks.at(-1);
    const hasActiveBlock =
      lastBlock?.kind === "reasoning" ||
      (lastBlock?.kind === "actions" && lastBlock.items.some(running));
    const showIdleThinking = !hasActiveBlock;
    return (
      <div className="space-y-1.5">
        <div role="status" className="space-y-1">
          <div className="flex items-center gap-2 text-xs text-slate-500 dark:text-muted">
            <span className="shrink-0 font-medium tabular-nums">
              {Number.isFinite(from) ? `Trabajando durante ${elapsed((now - from) / 1000)}` : "Trabajando…"}
            </span>
          </div>
          <div className="h-px w-full bg-slate-200 dark:bg-border" />
        </div>
        {blocks.map(renderBlock)}
        {showIdleThinking && (
          <div className="flex min-h-6 items-center gap-2 text-[0.8125rem] leading-relaxed text-slate-500 dark:text-muted">
            <Brain weight="regular" className="size-4 shrink-0" />
            <span className="shimmer-text">Pensando</span>
          </div>
        )}
      </div>
    );
  }
  const time = formatTime(endedAt);
  const summary = stopped
    ? (secs >= 1 ? `Lo paraste tras ${elapsed(secs)}` : "Lo paraste")
    : (secs >= 1 ? `Trabajó durante ${elapsed(secs)}` : "Trabajó");
  return (
    <div className="text-sm">
      <button type="button" aria-expanded={open} onClick={() => setOpen(!open)}
        className="flex w-full items-center gap-1.5 py-0.5 text-xs text-slate-500 hover:text-slate-900 dark:text-muted dark:hover:text-white">
        <span className="shrink-0 font-medium tabular-nums">{summary}</span>
        {open ? <CaretDown weight="regular" className="size-3 shrink-0 opacity-70" /> : <CaretRight weight="regular" className="size-3 shrink-0 opacity-70" />}
        <span className="flex-1" />
        {time && <span className="shrink-0 text-[0.6875rem] tabular-nums opacity-60">{time}</span>}
      </button>
      <div className="mt-1 h-px w-full bg-slate-200 dark:bg-border" />
      {open && blocks.length > 0 && <div className="mt-1.5 space-y-1.5">{blocks.map(renderBlock)}</div>}
    </div>
  );
}

// Reasoning shows a standalone reasoning block folded to its preview line.
export function Reasoning({ m }: { m: Message }) {
  const [open, setOpen] = useState(false);
  if (!m.text.trim()) return null;
  const preview = thoughtPreview(m.text) || (m.streaming ? "Pensando" : "Pensó");
  return (
    <div className="text-[0.8125rem] leading-relaxed text-slate-500 dark:text-muted">
      <button type="button" aria-expanded={open} onClick={() => setOpen(!open)}
        className="flex min-h-6 w-full min-w-0 items-center gap-2 text-left hover:text-slate-900 dark:hover:text-white">
        <Brain weight="regular" className="size-4 shrink-0" />
        <span className={`min-w-0 flex-1 truncate ${m.streaming ? "shimmer-text" : ""}`}>{preview}</span>
        {open ? <CaretDown weight="regular" className="size-3 shrink-0 opacity-70" /> : <CaretRight weight="regular" className="size-3 shrink-0 opacity-70" />}
      </button>
      {open && <ReasoningScroll items={[m]} />}
    </div>
  );
}
