import { ArrowCounterClockwise,CaretDown,Check,Copy,ListChecks,Stop,WarningCircle } from "@phosphor-icons/react";
import { memo,useEffect,useMemo,useRef,useState } from "react";
import { api,type Message } from "../../api/client";
import { Md,copyText } from "../../components/ui";
import { messageText,t } from "../../i18n/es";
import { events } from "../../lib/store";
import { HtmlCard,htmlRef } from "./html-card";
import { MemoryCitations,extractMemoryCitations } from "./memory-citation";
import { ModelChangeDivider,readModelChange } from "./model-change";
import { NativeQuestionCard,QuestionCard } from "./question-card";
import { Changes,QuestionStep,Reasoning,StepLine,Worked,WorkedBlock,layout } from "./steps";
import { rehypeFade,useSmoothText } from "./stream";
import { AgentMessage,agentNote } from "./subagents";
import { TodoCard,ToolCard,isAsk } from "./tool-card";
import { TurnChanges } from "./turn-changes";
import { SentFiles,UserMessageBody } from "./user-message";

// The look of the conversation, kept from Dory v1: user
// bubbles, answers as plain markdown, and each finished turn's work folded
// into one "Worked for…" line.

// msgBtn is a message action: on hover only the icon changes colour, no round background.
const msgBtn = "grid size-7 place-items-center text-slate-400 hover:text-slate-900 disabled:opacity-30 dark:hover:text-white";
export const iconBtn = "grid size-7 place-items-center rounded-full text-slate-400 hover:bg-slate-100 hover:text-slate-900 disabled:opacity-30 dark:hover:bg-raised dark:hover:text-white";

export const fullDate = new Intl.DateTimeFormat("es-ES", { weekday: "long", day: "numeric", month: "long", year: "numeric", hour: "2-digit", minute: "2-digit", second: "2-digit" });
export const shortTime = new Intl.DateTimeFormat("es-ES", { hour: "2-digit", minute: "2-digit" });

// Messages lays out a conversation; running is whether its last turn is live.
// Only messages that arrive after it mounts animate in.
// Messages off screen skip layout and paint (content-visibility), so a long
// conversation does not reflow whole while the side panel slides its width.
// They render visibly for two frames on mount so "auto" records each message's
// real height upfront; otherwise scrolling up would reveal real heights on the
// fly, shifting scrollHeight above the viewport and causing scroll stutters.
const measuring = { contentVisibility: "visible", containIntrinsicSize: "auto 160px" } as const;
const offscreen = { contentVisibility: "auto", containIntrinsicSize: "auto 160px" } as const;

export const Messages = memo(function Messages({ threadId, messages: all, running }: { threadId: string; messages: Message[]; running: boolean }) {
  const initial = useRef<Set<string> | null>(null);
  if (!initial.current) initial.current = new Set(all.map((m) => m.id));
  const [settled, setSettled] = useState(false);
  useEffect(() => {
    let f2 = 0;
    const f1 = requestAnimationFrame(() => { f2 = requestAnimationFrame(() => setSettled(true)); });
    return () => { cancelAnimationFrame(f1); cancelAnimationFrame(f2); };
  }, []);
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
  const itemStyle = settled ? offscreen : measuring;
  // The anchor names a place by turn and position, not by id: after each turn
  // Dory reloads the conversation from the harness's file with new ids, and
  // the reader must stay on the same message.
  let turn = 0, at = 0;
  const place = (m: Message | Worked | Changes) => { if ("role" in m && m.role === "user") { turn++; at = 0; } return `${turn}:${at++}`; };
  return <>{layout(messages, running).map((m) => <div key={m.id} data-anchor={place(m)} style={itemStyle}>{"changes" in m ? <TurnChanges threadId={threadId} turn={m.turn} /> : "worked" in m ? <WorkedBlock Entry={Entry} items={m.items} secs={m.secs} live={m.live} since={m.since} endedAt={m.endedAt} stopped={m.stopped} expanded={m.expanded} />
    : <Entry m={m} threadId={threadId} fresh={!initial.current?.has(m.id)} openQuestion={m.id === openQuestion} />}</div>)}</>;
});

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
      { const note = agentNote(m); if (note) return <AgentMessage note={note} />; }
      return (
        <div data-message className={`group flex flex-col items-end ${fresh ? "msg-in" : ""}`}>
          {threadId && <SentFiles threadId={threadId} detail={m.detail} />}
          {m.text && <UserMessageBody text={m.text} />}
          <MessageMeta m={m} threadId={threadId} />
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
      if (isAsk(m)) return openQuestion ? <QuestionCard m={m} /> : <QuestionStep m={m} />;
      if (htmlRef(m)) return <HtmlCard m={m} threadId={threadId} />;
      if (m.tool?.kind === "question") return openQuestion ? <NativeQuestionCard m={m} /> : <QuestionStep m={m} />;
      return m.tool ? <ToolCard m={m} /> : <StepLine run={[m]} />;
    case "plan":
      return m.tool?.kind === "todo" ? <TodoCard m={m} /> : <PlanCard m={m} />;
    case "question":
      return openQuestion ? <QuestionCard m={m} /> : <QuestionStep m={m} />;
    case "model-change":
      return <ModelChangeDivider change={readModelChange(m.detail)} />;
    case "interrupt":
    case "clear":
      return (
        <div role="separator" className="flex items-center gap-3 text-[0.6875rem] font-medium uppercase tracking-wide text-slate-400">
          <span className="h-px flex-1 bg-slate-200 dark:bg-border" />{m.role === "interrupt" && <Stop weight="fill" className="size-3" />}{messageText(m)}<span className="h-px flex-1 bg-slate-200 dark:bg-border" />
        </div>
      );
    default:
      // A failed turn reads as an alert, not as fine print.
      if (m.status === "failed") return (
        <div data-message role="alert" className="flex items-start gap-2.5 rounded-xl border border-red-200 bg-red-50 px-3.5 py-2.5 text-sm text-red-800 dark:border-red-900/60 dark:bg-red-950/30 dark:text-red-300">
          <WarningCircle weight="fill" className="mt-0.5 size-4 shrink-0" />
          <div className="min-w-0 flex-1 [&_*]:!text-inherit"><p className="font-medium">{m.title || t("turn.failed")}</p><Md>{messageText(m)}</Md></div>
        </div>
      );
      return (
        <div data-message className="text-xs text-slate-500 dark:text-muted">
          {m.title && <p className="font-medium">{m.title}</p>}
          <Md>{messageText(m)}</Md>
        </div>
      );
  }
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
  const { body, citations } = useMemo(() => extractMemoryCitations(text, m.streaming), [text, m.streaming]);
  if (!body && !citations.length) return null;
  return (
    <>
      {body && <Md serif rehypePlugins={fade}>{body}</Md>}
      <MemoryCitations citations={citations} />
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
export function MessageMeta({ m, threadId = "" }: { m: Message; threadId?: string }) {
  const [copied, setCopied] = useState(false);
  const [reverting, setReverting] = useState(false);
  // Going back to a user message restores the files of that turn, removes it
  // and what came after from the conversation and puts its text back in the box.
  const revert = async () => {
    if (!confirm("¿Volver a antes de este mensaje? Los ficheros vuelven a como estaban, y este mensaje y los siguientes desaparecen de la conversación.")) return;
    setReverting(true);
    try {
      const r = await api.revert(threadId, m.id);
      events.fillComposer.emit({ threadId, text: r.text });
      if (!r.files_restored) alert("La conversación ha vuelto atrás, pero este turno no tenía foto de los ficheros: no se han restaurado.");
    } catch (e) {
      alert(e instanceof Error ? e.message : String(e));
    } finally { setReverting(false); }
  };
  const at = m.created_at ? new Date(m.created_at) : undefined;
  const valid = at && !Number.isNaN(at.getTime());
  const copy = async () => {
    if (await copyText(m.text)) { setCopied(true); setTimeout(() => setCopied(false), 1500); }
  };
  return (
    <div className="mt-1 flex items-center gap-1 text-[0.6875rem] text-slate-400 transition-opacity sm:opacity-0 sm:group-hover:opacity-100 sm:focus-within:opacity-100">
      {valid && <time dateTime={m.created_at} title={fullDate.format(at)} className="cursor-default tabular-nums">{shortTime.format(at)}</time>}
      <button type="button" onClick={copy} title={copied ? "Copiado" : "Copiar mensaje"} aria-label="Copiar mensaje" className={msgBtn}>
        {copied ? <Check weight="regular" className="size-3.5" /> : <Copy className="size-3.5" />}
      </button>
      {threadId && m.role === "user" && (
        <button type="button" onClick={revert} disabled={reverting} title="Volver a antes de este mensaje" aria-label="Volver a antes de este mensaje" className={msgBtn}>
          <ArrowCounterClockwise weight="regular" className="size-3.5" />
        </button>
      )}
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
