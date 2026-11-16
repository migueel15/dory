import { useEffect, useLayoutEffect, useMemo, useRef, useState } from "react";
import { ArrowDown, Brain, CaretDown, CaretRight, Check, Copy, FileText, ListChecks, Stop, TerminalWindow, Wrench } from "@phosphor-icons/react";
import { api, type Attachment, type Message } from "../api/client";
import { copyText, defaultUrlTransform, Md } from "../components/ui";
import { TokenChip, tokenize, type TokenKind } from "../components/composer-tokens";
import { rehypeFade, useSmoothText } from "../components/stream";

// The look of the conversation, kept from Dory v1 (as in T3 Code): user
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
export function Messages({ threadId, messages, running }: { threadId: string; messages: Message[]; running: boolean }) {
  const initial = useRef<Set<string> | null>(null);
  if (!initial.current) initial.current = new Set(messages.map((m) => m.id));
  return <>{layout(messages, running).map((m) => "worked" in m ? <WorkedBlock key={m.id} items={m.items} secs={m.secs} live={m.live} since={m.since} />
    : <Entry key={m.id} m={m} threadId={threadId} fresh={!initial.current?.has(m.id)} />)}</>;
}

export function Entry({ m, threadId = "", fresh = false }: { m: Message; threadId?: string; fresh?: boolean }) {
  switch (m.role) {
    case "user":
      return (
        <div data-message className={`group flex flex-col items-end ${fresh ? "msg-in" : ""}`}>
          {threadId && <SentFiles threadId={threadId} detail={m.detail} />}
          {m.text && <div className="max-w-[85%] rounded-2xl rounded-br-md bg-slate-900 px-4 py-2.5 text-white shadow-sm dark:bg-accent [&_*]:!text-inherit [&_code]:!bg-white/15 [&_pre]:!border-white/15 [&_pre]:!bg-white/10 [&_pre_code]:!bg-transparent"><Md urlTransform={keepToken} components={tokenLinks}>{tokenize(m.text)}</Md></div>}
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
      return <StepLine run={[m]} />;
    case "plan":
      return <PlanCard m={m} />;
    case "interrupt":
      return (
        <div role="separator" className="flex items-center gap-3 text-[11px] font-medium uppercase tracking-wide text-slate-400">
          <span className="h-px flex-1 bg-slate-200 dark:bg-border" /><Stop weight="fill" className="size-3" />{m.text}<span className="h-px flex-1 bg-slate-200 dark:bg-border" />
        </div>
      );
    default:
      return (
        <div data-message className={`text-xs ${m.status === "failed" ? "text-red-600 dark:text-red-400" : "text-slate-500 dark:text-muted"}`}>
          {m.title && <p className="font-medium">{m.title}</p>}
          <Md>{m.text}</Md>
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

// SentFiles shows the files sent with a message, above its bubble.
function SentFiles({ threadId, detail }: { threadId: string; detail?: string }) {
  let files: Attachment[] = [];
  try { files = detail ? JSON.parse(detail) : []; } catch { return null; }
  if (!Array.isArray(files) || !files.length) return null;
  return (
    <div className="mb-1 flex max-w-[85%] flex-wrap justify-end gap-2">
      {files.map((f) => {
        const url = api.attachmentUrl(threadId, f.id);
        return f.mime?.startsWith("image/")
          ? <a key={f.id} href={url} target="_blank" rel="noreferrer" title={f.name} className="block overflow-hidden rounded-xl border border-slate-200 dark:border-border"><img src={url} alt={f.name} className="max-h-48 max-w-64 object-cover" /></a>
          : <a key={f.id} href={url} target="_blank" rel="noreferrer" className="inline-flex items-center gap-1.5 rounded-xl border border-slate-200 px-3 py-2 text-xs text-slate-600 hover:bg-slate-50 dark:border-border dark:text-text-secondary dark:hover:bg-raised"><FileText className="size-4" />{f.name}</a>;
      })}
    </div>
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
    <div className="mt-1 flex items-center gap-1 text-[11px] text-slate-400 transition-opacity sm:opacity-0 sm:group-hover:opacity-100 sm:focus-within:opacity-100">
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
const stepKind = (m: Message) => m.role === "assistant" ? "note" : m.role === "reasoning" ? "think" : shells.has(m.title ?? "") ? "cmd" : `tool:${m.title}`;

function stepRuns(items: Message[]): Message[][] {
  const out: Message[][] = [];
  for (const m of items) {
    if (m.role === "reasoning" && !m.text.trim() && !m.streaming) continue;
    const last = out.at(-1);
    if (last && stepKind(last[0]) === stepKind(m)) last.push(m);
    else out.push([m]);
  }
  return out;
}

const oneLine = (t = "") => t.replace(/[`*_#>]/g, "").replace(/\s+/g, " ").trim();

// StepLine is a run of steps of one kind: an icon and one line with the latest
// step. A click opens every step of the run in full.
export function StepLine({ run }: { run: Message[] }) {
  const [open, setOpen] = useState(false);
  const m = run[run.length - 1];
  const kind = stepKind(m);
  const live = running(m), bad = failed(m);
  const icon = kind === "think" ? <Brain weight="regular" className="size-4 shrink-0" /> : kind === "cmd" ? <TerminalWindow weight="regular" className="size-4 shrink-0" />
    : kind === "note" ? <span className="size-4 shrink-0" /> : <Wrench weight="regular" className="size-4 shrink-0" />;
  const line = kind === "think" ? (oneLine(m.text) || "Thinking…")
    : kind === "note" ? oneLine(m.text)
    : kind === "cmd" ? `${live ? "Running" : bad ? "Failed" : "Ran"} ${commandName(m.detail) || "command"}`
    : m.title || "Tool";
  const full = (x: Message) => x.role === "tool"
    ? <pre key={x.id} className="max-h-60 overflow-auto whitespace-pre-wrap break-all rounded bg-slate-100 p-2 font-mono text-[11px] dark:bg-raised">{[x.detail, x.text].filter(Boolean).join("\n\n")}</pre>
    : <div key={x.id} className="[&_*]:!text-inherit [&_p]:my-0.5"><Md>{x.text}</Md></div>;
  return (
    <div className={`text-[13px] leading-relaxed ${kind === "note" ? "text-slate-700 dark:text-text-secondary" : "text-slate-500 dark:text-muted"}`}>
      <button type="button" onClick={() => setOpen(!open)} aria-expanded={open}
        className="flex w-full min-w-0 items-center gap-2 text-left hover:text-slate-900 dark:hover:text-white">
        {icon}
        <span className={`min-w-0 flex-1 truncate ${live ? "motion-safe:animate-pulse" : ""} ${bad ? "text-red-600 dark:text-red-400" : ""}`}>{line}</span>
        {run.length > 1 && <span className="shrink-0 text-[11px] tabular-nums opacity-60">{run.length}</span>}
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
  const isWork = (m: Message) => m.role === "reasoning" || m.role === "tool" || m.role === "assistant";
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
    if (i > start && (m.role === "user" || (m.run_id && run && m.run_id !== run))) { flush(i, false); start = i; run = ""; }
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
        {stepRuns(items).map((r) => <StepLine key={r[0].id} run={r} />)}
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
        <div className="mt-0.5 border-l-2 border-slate-200 pl-3 text-[13px] leading-relaxed text-slate-500 dark:border-border dark:text-muted [&_*]:!text-inherit"><Md>{m.text}</Md></div>
      )}
    </div>
  );
}

// useFollow keeps a scroller pinned to the end unless the user scrolled up,
// and says when the end is far enough to offer a way back.
export function useFollow(deps: unknown[]) {
  const scroller = useRef<HTMLDivElement>(null);
  const pinned = useRef(true);
  const [away, setAway] = useState(false);
  useLayoutEffect(() => {
    const el = scroller.current;
    if (el && pinned.current) el.scrollTop = el.scrollHeight;
  }, deps); // eslint-disable-line react-hooks/exhaustive-deps
  const onScroll = () => {
    const el = scroller.current;
    if (!el) return;
    const distance = el.scrollHeight - el.scrollTop - el.clientHeight;
    pinned.current = distance < 40;
    setAway(distance > 200);
  };
  const follow = () => { pinned.current = true; const el = scroller.current; if (el) el.scrollTop = el.scrollHeight; };
  return { scroller, onScroll, follow, away };
}

// ToEnd is the floating "Ir al final" button.
export function ToEnd({ away, follow }: { away: boolean; follow: () => void }) {
  return (
    <div className="relative h-0">
      <button type="button" onClick={follow} aria-label="Ir al final" aria-hidden={!away} tabIndex={away ? 0 : -1}
        className={`absolute bottom-3 left-1/2 z-20 inline-flex -translate-x-1/2 items-center gap-1.5 transition-[translate,opacity] duration-200 ease-out ${away ? "translate-y-0 opacity-100" : "pointer-events-none translate-y-4 opacity-0"} rounded-full border border-slate-200 bg-white px-3 py-1.5 text-xs font-medium text-slate-600 shadow-md hover:bg-slate-50 hover:text-slate-900 dark:border-border-strong dark:bg-surface dark:text-text-secondary dark:hover:bg-raised dark:hover:text-white`}>
        <ArrowDown weight="bold" className="size-3.5" />Ir al final
      </button>
    </div>
  );
}
