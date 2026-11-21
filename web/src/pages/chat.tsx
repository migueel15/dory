import { useCallback, useEffect, useLayoutEffect, useMemo, useRef, useState } from "react";
import { ArrowDown, ArrowRight, Brain, ChatText, GitMerge, GitPullRequest, Lightning, Rocket, Question as QuestionIcon, CaretDown, CaretRight, Check, ClipboardText, Copy, FileText, ListChecks, Stop, TerminalWindow, WarningCircle, Wrench, X } from "@phosphor-icons/react";
import { api, type Attachment, type Message, type TurnModel } from "../api/client";
import { copyText, defaultUrlTransform, Md } from "../components/ui";
import { TokenChip, tokenize, type TokenKind } from "../components/composer-tokens";
import { claudeModelName } from "../components/model-label";
import { splitMessageQuotes } from "../components/message-quotes";
import { ModelLogo } from "../components/model-logo";
import { rehypeFade, useSmoothText } from "../components/stream";
import { CodeRefChip } from "../components/code-ref";
import { askQuestions, GroupCard, isAsk, TodoCard, ToolCard } from "../components/tool-card";
import { NativeQuestionCard, QuestionCard } from "../components/question-card";
import { Link } from "@tanstack/react-router";
import { parseAutoMessage, type AutoMessage } from "../lib/auto-message";
import { RefLink } from "../components/refs";
import { AgentMessage } from "../components/subagents";

// The look of the conversation, kept from Dory v1: user
// bubbles, answers as plain markdown, and each finished turn's work folded
// into one "Worked for…" line.

export const iconBtn = "grid size-7 place-items-center rounded-full text-slate-400 hover:bg-slate-100 hover:text-slate-900 disabled:opacity-30 dark:hover:bg-raised dark:hover:text-white";

// elapsed writes a duration the short way: "45s", "1m 59s", "1h 3m".
export const elapsed = (secs: number) => {
  const s = Math.max(0, Math.round(secs));
  if (s < 60) return `${s}s`;
  if (s < 3600) return `${Math.floor(s / 60)}m ${s % 60}s`;
  return `${Math.floor(s / 3600)}h ${Math.floor((s % 3600) / 60)}m`;
};

export const fullDate = new Intl.DateTimeFormat("es-ES", { weekday: "long", day: "numeric", month: "long", year: "numeric", hour: "2-digit", minute: "2-digit", second: "2-digit" });
export const shortTime = new Intl.DateTimeFormat("es-ES", { hour: "2-digit", minute: "2-digit" });

// Messages lays out a conversation; running is whether its last turn is live.
// Only messages that arrive after it mounts animate in.
// Messages off screen skip layout and paint (content-visibility), so a long
// conversation does not reflow whole while the side panel slides its width.
// "auto" remembers each one's last real height once painted.
const offscreen = { contentVisibility: "auto", containIntrinsicSize: "auto 160px" } as const;

// usePrepaint paints once each message that comes within 2000px of the
// visible area, so its real height is known before it scrolls into view and
// the page does not jump; afterwards it is skipped again with that height.
function usePrepaint() {
  const observer = useRef<IntersectionObserver | null>(null);
  useEffect(() => () => observer.current?.disconnect(), []);
  return useCallback((el: HTMLDivElement | null) => {
    if (!el) return;
    if (!observer.current) {
      observer.current = new IntersectionObserver((entries) => {
        for (const e of entries) {
          if (!e.isIntersecting) continue;
          const target = e.target as HTMLElement;
          observer.current?.unobserve(target);
          target.style.contentVisibility = "visible";
          requestAnimationFrame(() => requestAnimationFrame(() => { target.style.contentVisibility = "auto"; }));
        }
      }, { root: el.closest("[data-scroller]"), rootMargin: "2000px 0px" });
    }
    observer.current.observe(el);
  }, []);
}

export function Messages({ threadId, messages: all, running }: { threadId: string; messages: Message[]; running: boolean }) {
  const initial = useRef<Set<string> | null>(null);
  if (!initial.current) initial.current = new Set(all.map((m) => m.id));
  // The ask_user call holds the question and survives the resync from the
  // harness's session file; the question message Dory saved does not, so
  // when the call is there it is the one shown.
  const messages = useMemo(() => {
    const ms = latestTodos(all);
    return ms.some(isAsk) ? ms.filter((m) => m.role !== "question") : ms;
  }, [all]);
  // A question is open while nothing the user sent came after it.
  let openQuestion: string | undefined;
  for (const m of messages) openQuestion = m.role === "user" ? undefined : isAsk(m) ? (m.status === "running" ? m.id : undefined) : m.role === "question" ? m.id : openQuestion;
  const prepaint = usePrepaint();
  return <>{layout(messages, running).map((m) => <div key={m.id} ref={prepaint} style={offscreen}>{"worked" in m ? <WorkedBlock items={m.items} secs={m.secs} live={m.live} since={m.since} />
    : <Entry m={m} threadId={threadId} fresh={!initial.current?.has(m.id)} openQuestion={m.id === openQuestion} />}</div>)}</>;
}

// latestTodos keeps, of a turn's task lists, only the last: each update of the
// list is a new call, and the chat shows the list once, updated in place.
function latestTodos(ms: Message[]): Message[] {
  const last = new Map<string, string>();
  for (const m of ms) if (m.tool?.kind === "todo") last.set(m.run_id ?? "", m.id);
  if (!last.size) return ms;
  return ms.filter((m) => m.tool?.kind !== "todo" || last.get(m.run_id ?? "") === m.id);
}

export function Entry({ m, threadId = "", fresh = false, openQuestion = false }: { m: Message; threadId?: string; fresh?: boolean; openQuestion?: boolean }) {
  switch (m.role) {
    case "user":
      if (m.sender_thread_id) return <AgentMessage m={m} />;
      return (
        <div data-message className={`group flex flex-col items-end ${fresh ? "msg-in" : ""}`}>
          {threadId && <SentFiles threadId={threadId} detail={m.detail} />}
          {m.text && <UserMessageBody text={m.text} />}
          <MessageMeta m={m} />
        </div>
      );
    case "assistant":
      return (
        <div data-message className="group max-w-full pr-6">
          <Answer m={m} />
          {m.streaming && !m.text ? <Dots /> : !m.streaming && <MessageMeta m={m} />}
        </div>
      );
    case "reasoning":
      return <Reasoning m={m} />;
    case "tool":
      if (isAsk(m)) return <QuestionCard m={m} />;
      if (m.tool?.kind === "question") return <NativeQuestionCard m={m} />;
      return m.tool ? <ToolCard m={m} /> : <StepLine run={[m]} />;
    case "plan":
      return m.tool?.kind === "todo" ? <TodoCard m={m} /> : <PlanCard m={m} />;
    case "question":
      return <QuestionCard m={m} />;
    case "model-change":
      return <ModelChangeDivider change={readModelChange(m.detail)} />;
    case "interrupt":
    case "clear":
      return (
        <div role="separator" className="flex items-center gap-3 text-[0.6875rem] font-medium uppercase tracking-wide text-slate-400">
          <span className="h-px flex-1 bg-slate-200 dark:bg-border" />{m.role === "interrupt" && <Stop weight="fill" className="size-3" />}{m.text}<span className="h-px flex-1 bg-slate-200 dark:bg-border" />
        </div>
      );
    default:
      // A failed turn reads as an alert, not as fine print.
      if (m.status === "failed") return (
        <div data-message role="alert" className="flex items-start gap-2.5 rounded-xl border border-red-200 bg-red-50 px-3.5 py-2.5 text-sm text-red-800 dark:border-red-900/60 dark:bg-red-950/30 dark:text-red-300">
          <WarningCircle weight="fill" className="mt-0.5 size-4 shrink-0" />
          <div className="min-w-0 flex-1 [&_*]:!text-inherit"><p className="font-medium">{m.title || "El turno falló"}</p><Md>{m.text}</Md></div>
        </div>
      );
      return (
        <div data-message className="text-xs text-slate-500 dark:text-muted">
          {m.title && <p className="font-medium">{m.title}</p>}
          <Md>{m.text}</Md>
        </div>
      );
  }
}

type ModelChange = { from: TurnModel; to: TurnModel; handoff: boolean };
const providerLabels: Record<string, string> = { claude: "Claude Code", codex: "Codex", opencode: "OpenCode", antigravity: "Antigravity" };

function readModelChange(detail?: string): ModelChange | undefined {
  try {
    const change = JSON.parse(detail ?? "") as ModelChange | null;
    return change?.from && change.to ? change : undefined;
  } catch {
    return undefined;
  }
}

function transitionLabel(model: TurnModel, fallback: string) {
  const provider = providerLabels[model.provider] ?? model.provider;
  const id = model.reported_model || model.model;
  const name = id && id !== "default" ? claudeModelName(id) ?? id.replace(/^gpt-/, "GPT-") : fallback;
  return { provider, name };
}

export function ModelChangeDivider({ change, pending = false }: { change?: ModelChange; pending?: boolean }) {
  if (!change) return null;
  const from = transitionLabel(change.from, "Modelo anterior");
  const to = transitionLabel(change.to, "Modelo configurado");
  const caption = pending
    ? (change.handoff ? "Traspaso de contexto pendiente" : "Cambio de modelo pendiente")
    : change.handoff ? "Contexto transferido" : "";
  const rule = pending ? "border-t border-dashed border-slate-300 dark:border-border-strong" : "border-t border-slate-200 dark:border-border";
  return (
    <div role="separator"
      aria-label={`Cambio de modelo de ${from.provider} ${from.name} a ${to.provider} ${to.name}${caption ? `. ${caption}` : ""}`}
      className="my-5 flex items-center gap-3 text-center text-xs text-slate-600 dark:text-muted">
      <span className={`min-w-3 flex-1 ${rule}`} />
      <div className="min-w-0">
        <div className="flex flex-wrap items-center justify-center gap-x-2 gap-y-1">
          <span title={from.provider} className="inline-flex items-center gap-1">
            <ModelLogo driver={change.from.provider} />
            {from.name}
          </span>
          <ArrowRight weight="thin" aria-hidden="true" className="size-3.5 shrink-0" />
          <span title={to.provider} className="inline-flex items-center gap-1 font-medium text-slate-800 dark:text-text">
            <ModelLogo driver={change.to.provider} />
            {to.name}
          </span>
        </div>
        {caption && <p className="mt-1 text-[0.6875rem]">{caption}</p>}
      </div>
      <span className={`min-w-3 flex-1 ${rule}`} />
    </div>
  );
}

// Answer is the model's reply, rendered as in Claude: serif prose that, while
// it is written, comes in at a steady pace with each word fading in. Replies
// already finished when the chat opened show at once.
function Answer({ m }: { m: Message }) {
  const live = useRef(m.streaming);
  if (m.streaming) live.current = true;
  const text = useSmoothText(m.text, m.streaming);
  const seen = useRef<number[]>([]);
  const fade = useMemo(() => (live.current ? [rehypeFade(seen.current)] : []), [live.current]); // eslint-disable-line react-hooks/exhaustive-deps
  if (!text) return null;
  return <Md serif rehypePlugins={fade}>{text}</Md>;
}

// Objects of a sent message (quotes, files, skills, commands) show as chips.
const keepToken = (url: string) => (url.startsWith("tok:") || url.startsWith("file://") ? url : defaultUrlTransform(url));
const tokenLinks = {
  a: ({ href, children, node: _n, ...rest }: React.AnchorHTMLAttributes<HTMLAnchorElement> & { node?: unknown }) => {
    if (!href?.startsWith("tok:")) return <a href={href} target="_blank" rel="noreferrer" {...rest}>{children}</a>;
    const [, kind, raw] = href.split(":");
    const value = decodeURIComponent(raw ?? "");
    if (kind === "quote") {
      const q = (() => { try { return JSON.parse(value) as { label: string; body: string }; } catch { return { label: value, body: value }; } })();
      return <TokenChip kind="quote" label={q.label} title={q.body} />;
    }
    const label = kind === "path" ? value.replace(/\/+$/, "").split("/").pop() || value : value;
    return <TokenChip kind={kind as TokenKind} label={label} title={value} />;
  },
};

// MessageParts renders a user message, with its quotes as a bar on the left.
export function MessageParts({ text }: { text: string }) {
  return <>{splitMessageQuotes(text).map((part, index) => part.kind === "code" ? <CodeRefChip key={index} data={part.ref} /> : part.kind === "quote" ? (
    <blockquote key={index} className="min-w-0 whitespace-pre-wrap break-words border-l-2 border-slate-300 pl-2.5 text-sm leading-relaxed text-slate-600 dark:border-border-strong dark:text-muted">
      {part.text}
    </blockquote>
  ) : (
    // Same size and line height as the answers.
    <div key={index} className="[&>div]:!text-[1rem] [&>div]:!leading-[1.7]"><Md urlTransform={keepToken} components={tokenLinks}>{tokenize(part.text)}</Md></div>
  ))}</>;
}

// AUTO names each kind of message Dory writes for the user, with its icon.
const AUTO: Record<string, { label: string; hint: string; icon: typeof ClipboardText }> = {
  plan: { label: "Modo plan", hint: "definir la tarea antes de implementarla", icon: ClipboardText },
  implement: { label: "Implementar", hint: "trabajar la tarea en su rama", icon: Rocket },
  "issue-tasks": { label: "Tareas de la issue", hint: "partir la issue en tareas", icon: ListChecks },
  "pr-conflicts": { label: "Conflictos", hint: "resolver los conflictos de la pull request", icon: GitMerge },
  "pr-review": { label: "Revisión", hint: "revisar la pull request", icon: GitPullRequest },
  "pr-question": { label: "Pregunta sobre el código", hint: "", icon: GitPullRequest },
};

// AutoStart shows a message Dory wrote for the user: what it is for, a link
// to what it is about and the user's own words. The context the agent got
// folds below.
function AutoStart({ msg }: { msg: AutoMessage }) {
  const [open, setOpen] = useState(false);
  const meta = AUTO[msg.kind] ?? { label: "Mensaje de Dory", hint: "", icon: Lightning };
  const Icon = meta.icon;
  const { task, title, repo, pr, issue, path, start, end } = msg.attrs;
  const ref = pr ? `#${pr}` : issue ? `#${issue}` : "";
  const where = path ? `${path}${start && start !== "0" ? `:${start}${end && end !== start ? `-${end}` : ""}` : ""}` : "";
  return (
    <div className="w-full max-w-[85%] overflow-hidden rounded-2xl rounded-br-md border border-slate-200 bg-white dark:border-border dark:bg-surface">
      <div className="flex min-w-0 items-center gap-2 border-b border-slate-200 px-4 py-2 text-sm dark:border-border">
        <Icon className="size-4 shrink-0 text-accent dark:text-accent-text" aria-hidden="true" />
        <span className="shrink-0 font-semibold">{meta.label}</span>
        <span className="min-w-0 truncate text-slate-500 dark:text-muted">{title ? `· ${title}` : where ? `· ${where}` : meta.hint ? `· ${meta.hint}` : ""}</span>
        <span className="ml-auto shrink-0 text-xs text-slate-500 dark:text-muted">
          {task && <Link to="/tasks" search={{ task } as never} className="hover:underline">Ver tarea</Link>}
          {repo && ref && <RefLink href={`https://github.com/${repo}/${pr ? "pull" : "issues"}/${pr ?? issue}`} className="hover:underline">{repo}{ref}</RefLink>}
        </span>
      </div>
      {msg.body.trim() && <div className="px-4 py-2.5"><MessageParts text={msg.body} /></div>}
      <div className="border-t border-slate-200 px-4 py-1.5 dark:border-border">
        <button type="button" aria-expanded={open} onClick={() => setOpen(!open)} className="flex items-center gap-1 text-xs text-slate-500 hover:text-slate-900 dark:text-muted dark:hover:text-white">
          {open ? <CaretDown className="size-3" /> : <CaretRight className="size-3" />}Contexto enviado al agente
        </button>
        {open && <div className="mt-1 pb-1 text-[0.8125rem] text-slate-600 dark:text-muted [&_*]:!text-inherit"><Md>{msg.context}</Md></div>}
      </div>
    </div>
  );
}

function UserMessageBody({ text }: { text: string }) {
  const auto = parseAutoMessage(text);
  if (auto) return <AutoStart msg={auto} />;
  const parts = splitMessageQuotes(text);
  const hasQuotes = parts.some((part) => part.kind !== "text");
  if (!hasQuotes) {
    return (
      <div className="max-w-[85%] rounded-2xl rounded-br-md bg-slate-900 px-4 py-2.5 text-white shadow-sm dark:bg-accent [&_*]:!text-inherit [&_code]:!bg-white/15 [&_pre]:!border-white/15 [&_pre]:!bg-white/10 [&_pre_code]:!bg-transparent">
        <MessageParts text={text} />
      </div>
    );
  }
  return (
    <div className="flex max-w-[85%] flex-col gap-1.5 rounded-2xl rounded-br-md bg-slate-100 px-4 py-2.5 dark:bg-raised">
      <MessageParts text={text} />
    </div>
  );
}

// SentFiles shows the files sent with a message, above its bubble.
function SentFiles({ threadId, detail }: { threadId: string; detail?: string }) {
  const [image, setImage] = useState<Attachment>();
  const dialog = useRef<HTMLDialogElement>(null);
  useEffect(() => {
    if (image && dialog.current && !dialog.current.open) dialog.current.showModal();
  }, [image]);
  let files: Attachment[] = [];
  try { files = detail ? JSON.parse(detail) : []; } catch { return null; }
  if (!Array.isArray(files) || !files.length) return null;
  return (
    <>
      <div className="mb-1 flex max-w-[85%] flex-wrap justify-end gap-2">
        {files.map((f) => {
          const url = api.attachmentUrl(threadId, f.id);
          return f.mime?.startsWith("image/")
            ? <button key={f.id} type="button" onClick={() => setImage(f)} title={f.name} aria-label={`Ampliar ${f.name}`} className="max-w-full cursor-zoom-in overflow-hidden rounded-xl border border-slate-200 dark:border-border">
              <img src={url} alt={f.name} className="max-h-48 w-auto max-w-full sm:max-w-64" />
            </button>
            : <a key={f.id} href={url} target="_blank" rel="noreferrer" className="inline-flex items-center gap-1.5 rounded-xl border border-slate-200 px-3 py-2 text-xs text-slate-600 hover:bg-slate-50 dark:border-border dark:text-text-secondary dark:hover:bg-raised"><FileText className="size-4" />{f.name}</a>;
        })}
      </div>
      {image && (
        <dialog ref={dialog} aria-label={image.name} onClose={() => setImage(undefined)}
          onClick={(event) => { if (event.target === event.currentTarget) event.currentTarget.close(); }}
          className="m-auto max-h-[92dvh] max-w-[94vw] overflow-hidden rounded-xl bg-white text-slate-900 backdrop:bg-black/75 dark:bg-surface dark:text-text">
          <div className="flex items-center justify-between gap-4 px-3 py-2">
            <span className="min-w-0 truncate text-sm">{image.name}</span>
            <button type="button" aria-label="Cerrar imagen" onClick={() => dialog.current?.close()} className="grid size-7 shrink-0 place-items-center text-slate-400 hover:text-slate-900 dark:hover:text-white"><X weight="regular" className="size-4" /></button>
          </div>
          <img src={api.attachmentUrl(threadId, image.id)} alt={image.name} className="max-h-[82dvh] max-w-full object-contain" />
        </dialog>
      )}
    </>
  );
}

function Dots() {
  return (
    <span className="mt-1 inline-flex gap-1" aria-label="Escribiendo">
      {[0, 150, 300].map((d) => <span key={d} className="size-1.5 animate-bounce rounded-full bg-slate-400" style={{ animationDelay: `${d}ms` }} />)}
    </span>
  );
}

// MessageMeta shows, on hover, when a message was sent and a button to copy it.
export function MessageMeta({ m }: { m: Message }) {
  const [copied, setCopied] = useState(false);
  const at = m.created_at ? new Date(m.created_at) : undefined;
  const valid = at && !Number.isNaN(at.getTime());
  const copy = async () => {
    if (await copyText(m.text)) { setCopied(true); setTimeout(() => setCopied(false), 1500); }
  };
  return (
    <div className="mt-1 flex items-center gap-1 text-[0.6875rem] text-slate-400 transition-opacity sm:opacity-0 sm:group-hover:opacity-100 sm:focus-within:opacity-100">
      {valid && <time dateTime={m.created_at} title={fullDate.format(at)} className="cursor-default tabular-nums">{shortTime.format(at)}</time>}
      <button type="button" onClick={copy} title={copied ? "Copiado" : "Copiar mensaje"} aria-label="Copiar mensaje" className={iconBtn}>
        {copied ? <Check weight="regular" className="size-3.5" /> : <Copy className="size-3.5" />}
      </button>
    </div>
  );
}

// PlanCard shows a plan as a readable document, folded to a preview when long.
export function PlanCard({ m }: { m: Message }) {
  const [open, setOpen] = useState(false);
  const [copied, setCopied] = useState(false);
  const text = m.text || m.detail || "";
  const long = text.length > 1200;
  const title = text.match(/^#\s+(.+)$/m)?.[1];
  const copy = async () => { if (await copyText(text)) { setCopied(true); setTimeout(() => setCopied(false), 1500); } };
  return (
    <article className="group overflow-hidden rounded-2xl border border-slate-200 bg-white shadow-sm dark:border-border dark:bg-surface">
      <header className="flex items-center gap-2 border-b border-slate-200 bg-slate-50 px-4 py-2.5 dark:border-border dark:bg-raised">
        <ListChecks className="size-4 shrink-0 text-slate-500 dark:text-muted" />
        <span className="text-xs font-semibold uppercase tracking-wide text-slate-500 dark:text-muted">Plan propuesto</span>
        {m.streaming && <span className="size-3 animate-spin rounded-full border-2 border-current border-t-transparent text-slate-400" />}
        <button type="button" onClick={copy} title={copied ? "Copiado" : "Copiar plan en markdown"} aria-label="Copiar plan" className={`ml-auto ${iconBtn}`}>
          {copied ? <Check weight="regular" className="size-3.5" /> : <Copy className="size-3.5" />}
        </button>
      </header>
      <div className={`relative px-5 py-4 ${long && !open ? "max-h-96 overflow-hidden" : ""}`}>
        <Md serif>{planMarkdown(text)}</Md>
        {long && !open && <div className="pointer-events-none absolute inset-x-0 bottom-0 h-24 bg-gradient-to-t from-white dark:from-surface" />}
      </div>
      {long && (
        <button type="button" onClick={() => setOpen(!open)} className="flex w-full items-center justify-center gap-1 border-t border-slate-200 py-2 text-xs font-medium text-slate-500 hover:bg-slate-50 hover:text-slate-900 dark:border-border dark:text-muted dark:hover:bg-raised dark:hover:text-white">
          {open ? <>Contraer <CaretDown weight="regular" className="size-3 rotate-180" /></> : <>Ver plan completo{title ? ` · ${title}` : ""} <CaretDown weight="regular" className="size-3" /></>}
        </button>
      )}
    </article>
  );
}

// planMarkdown turns the harness's "[x] step" lines into a markdown checklist.
const planMarkdown = (t: string) => t.replace(/^\[(x| |~)\]\s*/gm, (_, s) => (s === "x" ? "- [x] " : "- [ ] "));

const running = (m: Message) => m.streaming || (m.role === "tool" && !["completed", "failed", "error", "declined", "interrupted", "cancelled"].includes(m.status ?? ""));
const failed = (m: Message) => m.status === "failed" || m.status === "error";

// commandName is the program a shell command runs: "cd x && sed -i …" → "sed".
const commandName = (cmd = "") => {
  for (const part of cmd.split(/&&|\|\||;|\n/)) {
    const words = part.trim().split(/\s+/).filter((w) => w && !/^\w+=/.test(w));
    if (words.length && words[0] !== "cd") return words[0].replace(/^.*\//, "");
  }
  return "";
};

const shells = new Set(["Bash", "command execution", "exec_command", "shell", "bash"]);

// stepKind says which steps share a line: consecutive thoughts, commands,
// notes or calls to the same tool show as one line that updates in place.
// Calls in Dory's terms (m.tool) group only reads and searches; each other
// call is its own card.
const stepKind = (m: Message) => m.role === "assistant" ? "note" : m.role === "reasoning" ? "think"
  : m.tool ? (m.tool.kind === "read" || m.tool.kind === "search" ? `group:${m.tool.kind}` : `call:${m.id}`)
  : shells.has(m.title ?? "") ? "cmd" : `tool:${m.title}`;

function stepRuns(items: Message[]): Message[][] {
  const out: Message[][] = [];
  for (const m of items) {
    if (m.role === "reasoning" && !m.text.trim() && !m.streaming) continue;
    const last = out.at(-1);
    if (last && !isQuestion(m) && !isQuestion(last[0]) && stepKind(last[0]) === stepKind(m)) last.push(m);
    else out.push([m]);
  }
  return out;
}

const isQuestion = (m: Message) => isAsk(m) || m.tool?.kind === "question" || m.role === "question";

// QuestionStep is a question of the agent among the steps: one line, and the
// card with the questions and answers when opened.
function QuestionStep({ m }: { m: Message }) {
  const [open, setOpen] = useState(false);
  const qs = isAsk(m) ? askQuestions(m) : m.tool?.questions ?? [];
  const first = qs[0]?.question ?? m.text;
  const answered = qs.some((q) => q.answer?.length);
  return (
    <div className="text-[0.8125rem] leading-relaxed text-slate-500 dark:text-muted">
      <button type="button" onClick={() => setOpen(!open)} aria-expanded={open}
        className="flex w-full min-w-0 items-center gap-2 text-left hover:text-slate-900 dark:hover:text-white">
        <QuestionIcon weight="regular" className="size-4 shrink-0" />
        <span className="min-w-0 flex-1 truncate">{answered ? "Respondida" : "Pregunta"}: {oneLine(first)}{qs.length > 1 ? ` (+${qs.length - 1})` : ""}</span>
      </button>
      {open && <div className="mb-1 ml-6 mt-1.5">{isAsk(m) || m.role === "question" ? <QuestionCard m={m} /> : <NativeQuestionCard m={m} />}</div>}
    </div>
  );
}

const oneLine = (t = "") => t.replace(/[`*_#>]/g, "").replace(/\s+/g, " ").trim();

// StepLine is a run of steps of one kind: an icon and one line with the latest
// step. A click opens every step of the run in full.
export function StepLine({ run }: { run: Message[] }) {
  if (isQuestion(run[0])) return <QuestionStep m={run[0]} />;
  if (run[0].tool) return run[0].tool.kind === "read" || run[0].tool.kind === "search" ? <GroupCard run={run} /> : <ToolCard m={run[0]} />;
  return <LegacyStep run={run} />;
}

// LegacyStep draws steps saved before calls carried their Dory kind.
function LegacyStep({ run }: { run: Message[] }) {
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
type Worked = { worked: true; id: string; items: Message[]; secs: number; live?: boolean; since?: string };

// layout lays a conversation out turn by turn. In a finished turn, everything
// between the user's message and the final answer folds into one "Worked
// for…" entry; the turn still running shows its steps as they come.
function layout(ms: Message[], live: boolean): (Message | Worked)[] {
  const out: (Message | Worked)[] = [];
  // A question is one more step: the composer is where it is answered.
  const isWork = (m: Message) => m.role === "reasoning" || m.role === "tool" || m.role === "question" || m.role === "assistant";
  let start = 0;
  const flush = (end: number, last: boolean) => {
    const seg = ms.slice(start, end);
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
    let answer = -1;
    for (let i = seg.length - 1; i >= 0; i--) if (seg[i].role === "assistant") { answer = i; break; }
    const work = seg.filter((m, i) => isWork(m) && i !== answer);
    if (!work.length) { out.push(...seg); return; }
    const from = Date.parse(head?.created_at || work[0].created_at);
    const to = Date.parse((answer >= 0 ? seg[answer] : work[work.length - 1]).created_at);
    out.push({ worked: true, id: `worked:${work[0].id}`, items: work, secs: Number.isFinite(from) && Number.isFinite(to) ? (to - from) / 1000 : 0 });
    out.push(...seg.filter((m, i) => !(isWork(m) && i !== answer)));
  };
  let run = "";
  ms.forEach((m, i) => {
    if (i > start && (m.role === "user" || m.role === "model-change" || m.role === "clear" || (m.run_id && run && m.run_id !== run))) { flush(i, false); start = i; run = ""; }
    if (m.run_id) run = m.run_id;
  });
  flush(ms.length, true);
  return out;
}

// WorkedBlock is a turn's work: while it runs, a ticking "Working for 12s"
// line and its steps; once it ends, a folded "Worked for 1m 59s".
function WorkedBlock({ items, secs, live = false, since }: { items: Message[]; secs: number; live?: boolean; since?: string }) {
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
function Reasoning({ m }: { m: Message }) {
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

// useFollow keeps a scroller pinned to the end unless the user scrolled up,
// and says when the end is far enough to offer a way back.
export function useFollow(deps: unknown[]) {
  const scroller = useRef<HTMLDivElement>(null);
  const pinned = useRef(true);
  // Where the last scroll left the view: only a move up is the user leaving the end.
  const lastTop = useRef(0);
  // True while "Ir al final" glides down, so its own steps do not unpin.
  const chasing = useRef(false);
  const [away, setAway] = useState(false);
  const toEnd = () => {
    const el = scroller.current;
    if (el) { el.scrollTop = el.scrollHeight; lastTop.current = el.scrollTop; }
  };
  useLayoutEffect(() => {
    if (pinned.current) toEnd();
  }, deps); // eslint-disable-line react-hooks/exhaustive-deps
  // Content also grows without new messages (code highlighted late, a step
  // opened, the composer taller): keep the end in view while pinned.
  useEffect(() => {
    const el = scroller.current;
    if (!el) return;
    const ro = new ResizeObserver(() => { if (pinned.current) { chasing.current = false; toEnd(); } });
    const watch = () => { ro.disconnect(); ro.observe(el); for (const c of Array.from(el.children)) ro.observe(c); };
    watch();
    const mo = new MutationObserver(watch);
    mo.observe(el, { childList: true });
    return () => { ro.disconnect(); mo.disconnect(); };
  }, []);
  // Only the user's own gesture leaves the end: the browser also moves the
  // view by itself (heights of messages painted late, scroll anchoring), and
  // those moves must not unpin.
  const touched = useRef(0);
  const held = useRef(false);
  useEffect(() => {
    const el = scroller.current;
    if (!el) return;
    const mark = () => { touched.current = Date.now(); chasing.current = false; };
    const wheel = (e: WheelEvent) => { if (e.deltaY < 0) mark(); };
    const key = (e: KeyboardEvent) => { if (["ArrowUp", "PageUp", "Home", " "].includes(e.key)) mark(); };
    const pointer = (e: PointerEvent) => { if (e.target === el) { held.current = true; mark(); } }; // the scrollbar
    const release = () => { held.current = false; };
    window.addEventListener("pointerup", release);
    el.addEventListener("wheel", wheel, { passive: true });
    el.addEventListener("touchmove", mark, { passive: true });
    el.addEventListener("keydown", key);
    el.addEventListener("pointerdown", pointer);
    return () => { el.removeEventListener("wheel", wheel); el.removeEventListener("touchmove", mark); el.removeEventListener("keydown", key); el.removeEventListener("pointerdown", pointer); window.removeEventListener("pointerup", release); };
  }, []);
  const onScroll = () => {
    const el = scroller.current;
    if (!el) return;
    const distance = el.scrollHeight - el.scrollTop - el.clientHeight;
    const up = el.scrollTop < lastTop.current - 1;
    lastTop.current = el.scrollTop;
    if (distance < 40) { pinned.current = true; chasing.current = false; }
    else if (up && !chasing.current && (held.current || Date.now() - touched.current < 1000)) pinned.current = false;
    else if (pinned.current) toEnd(); // the browser moved the view; go back to the end
    setAway(!pinned.current && distance > 200);
  };
  const follow = (smooth = false) => {
    pinned.current = true;
    const el = scroller.current;
    if (!el) return;
    const glide = smooth && !window.matchMedia("(prefers-reduced-motion: reduce)").matches;
    chasing.current = glide;
    el.scrollTo({ top: el.scrollHeight, behavior: glide ? "smooth" : "auto" });
  };
  return { scroller, onScroll, follow, away };
}

// ToEnd is the floating "Ir al final" button.
export function ToEnd({ away, follow }: { away: boolean; follow: (smooth?: boolean) => void }) {
  return (
    <div className="relative h-0">
      <button type="button" onClick={() => follow(true)} aria-label="Ir al final" aria-hidden={!away} tabIndex={away ? 0 : -1}
        className={`absolute bottom-3 left-1/2 z-20 inline-flex -translate-x-1/2 items-center gap-1.5 transition-[translate,opacity] duration-200 ease-out ${away ? "translate-y-0 opacity-100" : "pointer-events-none translate-y-4 opacity-0"} rounded-full border border-slate-200 bg-white px-3 py-1.5 text-xs font-medium text-slate-600 shadow-md hover:bg-slate-50 hover:text-slate-900 dark:border-border-strong dark:bg-surface dark:text-text-secondary dark:hover:bg-raised dark:hover:text-white`}>
        <ArrowDown weight="bold" className="size-3.5" />Ir al final
      </button>
    </div>
  );
}
