import { Link as RouterLink } from "@tanstack/react-router";
import { ArrowBendUpLeft, ArrowCounterClockwise, CaretDown, ChatCircle, Check, CheckCircle, Clock, Code, Eye, File, Flag, GitBranch, GitMerge, GitPullRequest, GithubLogo, PencilSimple, Plus, RadioButton, User, X, type Icon as PhosphorIcon, type IconWeight } from "@phosphor-icons/react";
import { defaultUrlTransform, Md, Textarea } from "./ui";
import { createContext, useContext, useEffect, useLayoutEffect, useRef, useState, type MutableRefObject, type ReactNode, type Ref, type TextareaHTMLAttributes } from "react";

// Phosphor icons shared by the GitHub views.
export const icons = {
  check: Check,
  comment: ChatCircle,
  issue: RadioButton,
  reply: ArrowBendUpLeft,
  x: X,
  plus: Plus,
  chevron: CaretDown,
  code: Code,
  merge: GitMerge,
  edit: PencilSimple,
  draft: GitPullRequest,
  ready: CheckCircle,
  reopen: ArrowCounterClockwise,
  user: User,
  ci: Clock,
  eye: Eye,
  clock: Clock,
  flag: Flag,
  file: File,
  branch: GitBranch,
  pull: GitPullRequest,
};

// Line-only glyphs have no inner area, so the app-wide duotone tint would
// only paint a square behind them: they stay regular.
const lineGlyphs = new Set<PhosphorIcon>([Check, X, Plus, CaretDown, Code, GitMerge, GitBranch, ArrowBendUpLeft, ArrowCounterClockwise]);

export function Icon({ i: Glyph, className = "size-4", weight }: { i: PhosphorIcon; className?: string; weight?: IconWeight }) {
  return <Glyph className={className} weight={weight ?? (lineGlyphs.has(Glyph) ? "regular" : undefined)} aria-hidden="true" />;
}

export function GithubMark({ className = "size-5" }: { className?: string }) {
  return <GithubLogo className={className} weight="fill" aria-hidden="true" />;
}

// Tip gives its child a tooltip; the text is drawn by the app's TooltipHost.
export function Tip({ text, children }: { text: string; children: ReactNode }) {
  return <span className="inline-flex" title={text}>{children}</span>;
}

const tones = {
  plain: "border-slate-200 bg-white text-slate-600 hover:bg-slate-100 hover:text-slate-900 dark:border-border dark:bg-surface dark:text-text-secondary dark:hover:bg-raised dark:hover:text-white",
  active: "border-blue-300 bg-blue-50 text-blue-700 dark:border-accent-border dark:bg-accent-subtle dark:text-accent-text",
  green: "border-green-300 bg-green-50 text-green-700 hover:bg-green-100 dark:border-green-800 dark:bg-green-950 dark:text-green-300 dark:hover:bg-green-900",
  red: "border-slate-200 bg-white text-slate-500 hover:border-red-300 hover:bg-red-50 hover:text-red-600 dark:border-border dark:bg-surface dark:hover:border-red-800 dark:hover:bg-red-950 dark:hover:text-red-400",
};

// IconButton is a square button whose meaning lives in its icon and tooltip.
// useArm makes an action need a second click: the first arms it for a few seconds.
export function useArm(run: () => void, ms = 3000): [boolean, () => void] {
  const [armed, setArmed] = useState(false);
  useEffect(() => {
    if (!armed) return;
    const t = setTimeout(() => setArmed(false), ms);
    return () => clearTimeout(t);
  }, [armed, ms]);
  return [armed, () => { if (armed) { setArmed(false); run(); } else setArmed(true); }];
}

// IconButton with confirm asks for a second click instead of opening a dialog.
export function IconButton({ icon, tip, onClick, disabled, tone = "plain", confirm = false }: { icon: PhosphorIcon; tip: string; onClick: () => void; disabled?: boolean; tone?: keyof typeof tones; confirm?: boolean }) {
  const [armed, click] = useArm(onClick);
  const ask = confirm && armed;
  return (
    <Tip text={ask ? "Pulsa otra vez para confirmar" : tip}>
      <button type="button" aria-label={ask ? `Confirmar: ${tip}` : tip} disabled={disabled} onClick={confirm ? click : onClick} className={`flex h-9 min-w-9 items-center justify-center rounded-lg border text-sm font-medium shadow-sm transition-all duration-200 ease-out motion-reduce:transition-none disabled:opacity-40 ${ask ? "px-2.5" : "px-0"} ${tones[tone]}`}>
        <Icon i={icon} />
        {confirm && <span aria-hidden={!ask} className={`overflow-hidden whitespace-nowrap transition-all duration-200 ease-out motion-reduce:transition-none ${ask ? "ml-1.5 max-w-28 opacity-100" : "ml-0 max-w-0 opacity-0"}`}>¿Confirmar?</span>}
      </button>
    </Tip>
  );
}

// Menu opens a floating panel under its trigger and closes on outside click or Escape.
export function Menu({ trigger, children, align = "left" }: { trigger: (open: boolean, toggle: () => void) => ReactNode; children: (close: () => void) => ReactNode; align?: "left" | "right" }) {
  const [open, setOpen] = useState(false);
  const [up, setUp] = useState(false);
  const box = useRef<HTMLDivElement>(null);
  const panel = useRef<HTMLDivElement>(null);
  // Open upwards when the panel would not fit below the trigger.
  useLayoutEffect(() => {
    if (!open || !box.current || !panel.current) return;
    const t = box.current.getBoundingClientRect(), h = panel.current.offsetHeight + 8;
    setUp(t.bottom + h > window.innerHeight && t.top - h > 0);
  }, [open]);
  useEffect(() => {
    if (!open) return;
    const away = (e: MouseEvent) => !box.current?.contains(e.target as Node) && setOpen(false);
    const esc = (e: KeyboardEvent) => e.key === "Escape" && setOpen(false);
    document.addEventListener("mousedown", away);
    document.addEventListener("keydown", esc);
    return () => { document.removeEventListener("mousedown", away); document.removeEventListener("keydown", esc); };
  }, [open]);
  return (
    <div ref={box} className="relative inline-flex">
      {trigger(open, () => setOpen(!open))}
      {open && (
        <div ref={panel} className={`absolute z-20 ${up ? "bottom-full mb-1.5" : "top-full mt-1.5"} min-w-56 overflow-hidden rounded-xl border border-slate-200 bg-white p-1 shadow-xl dark:border-border dark:bg-surface ${align === "right" ? "right-0" : "left-0"}`}>
          {children(() => setOpen(false))}
        </div>
      )}
    </div>
  );
}

export function MenuItem({ onClick, children }: { onClick: () => void; children: ReactNode }) {
  return (
    <button type="button" onClick={onClick} className="flex w-full items-center gap-2 rounded-lg px-2.5 py-2 text-left text-sm text-slate-700 hover:bg-slate-100 dark:text-text dark:hover:bg-raised">
      {children}
    </button>
  );
}

export const avatarUrl = (login: string) => `https://github.com/${login}.png?size=48`;

type U = { id: string; login: string };

// UserPicker is a round "+" button that opens a list of people with avatars.
export function UserPicker({ tip, title, users, note, onPick, disabled, extra }: { tip: string; title: string; users: U[]; note?: (u: U) => string | undefined; onPick: (u: U) => void; disabled?: boolean; extra?: (close: () => void) => ReactNode }) {
  return (
    <Menu
      trigger={(open, toggle) => (
        <Tip text={tip}>
          <button type="button" aria-label={tip} disabled={disabled} onClick={toggle} className={`grid size-6 place-items-center rounded-full border border-dashed transition-colors disabled:opacity-40 ${open ? "border-blue-500 dark:border-accent text-blue-600 dark:text-accent-text" : "border-slate-400 text-slate-500 hover:border-blue-500 dark:hover:border-accent hover:text-blue-600 dark:hover:text-accent-text"}`}>
            <Icon i={icons.plus} className="size-3.5" />
          </button>
        </Tip>
      )}
    >
      {(close) => (
        <>
          <div className="px-2.5 pb-1 pt-1.5 text-xs font-semibold uppercase tracking-wide text-slate-400">{title}</div>
          {extra?.(close)}
          {users.map((u) => (
            <MenuItem key={u.id} onClick={() => { close(); onPick(u); }}>
              <img src={avatarUrl(u.login)} alt="" className="size-6 rounded-full bg-slate-200 dark:bg-hover" />
              <span className="flex-1">{u.login}</span>
              {note?.(u) && <span className="text-xs text-slate-400">{note(u)}</span>}
            </MenuItem>
          ))}
          {!users.length && !extra && <div className="px-2.5 py-2 text-sm text-slate-400">No hay nadie más</div>}
        </>
      )}
    </Menu>
  );
}

type Ask = { title: string; text: string; ok: string; danger?: boolean; run: () => void };

// useConfirm returns a function that asks before running an action and the
// dialog to render; Escape or a click outside cancels it.
export function useConfirm(): [(a: Ask) => void, ReactNode] {
  const [ask, setAsk] = useState<Ask>();
  useEffect(() => {
    if (!ask) return;
    const esc = (e: KeyboardEvent) => e.key === "Escape" && setAsk(undefined);
    document.addEventListener("keydown", esc);
    return () => document.removeEventListener("keydown", esc);
  }, [ask]);
  const dialog = ask && (
    <div role="dialog" aria-modal="true" aria-label={ask.title} onMouseDown={(e) => e.target === e.currentTarget && setAsk(undefined)} className="fixed inset-0 z-50 grid place-items-center p-4">
      <div className="notif-in w-full max-w-sm rounded-xl border border-slate-200 bg-white p-5 shadow-2xl dark:border-border dark:bg-surface">
        <h2 className="text-base font-semibold">{ask.title}</h2>
        <p className="mt-1.5 text-sm text-slate-600 dark:text-text-secondary">{ask.text}</p>
        <div className="mt-5 flex justify-end gap-2">
          <button type="button" onClick={() => setAsk(undefined)} className="rounded-lg px-3 py-1.5 text-sm font-medium text-slate-600 hover:bg-slate-100 dark:text-text-secondary dark:hover:bg-raised">Cancelar</button>
          <button type="button" autoFocus onClick={() => { ask.run(); setAsk(undefined); }} className={`rounded-lg px-3 py-1.5 text-sm font-medium text-white shadow-sm ${ask.danger ? "bg-red-600 hover:bg-red-700" : "bg-slate-900 hover:bg-slate-700 dark:bg-accent dark:text-white dark:hover:bg-accent-hover"}`}>{ask.ok}</button>
        </div>
      </div>
    </div>
  );
  return [setAsk, dialog];
}

// caretXY measures where the caret of a textarea is, relative to the textarea,
// by copying its text up to the caret into a hidden element with the same style.
function caretXY(el: HTMLTextAreaElement, pos: number) {
  const div = document.createElement("div");
  const css = getComputedStyle(el);
  for (const p of ["boxSizing", "width", "fontFamily", "fontSize", "fontWeight", "lineHeight", "letterSpacing", "paddingTop", "paddingRight", "paddingBottom", "paddingLeft", "borderTopWidth", "borderRightWidth", "borderBottomWidth", "borderLeftWidth", "tabSize"] as const) div.style[p] = css[p];
  Object.assign(div.style, { position: "absolute", visibility: "hidden", whiteSpace: "pre-wrap", overflowWrap: "break-word", top: "0", left: "-9999px" });
  div.textContent = el.value.slice(0, pos);
  const mark = document.createElement("span");
  mark.textContent = "​";
  div.appendChild(mark);
  document.body.appendChild(div);
  const xy = { x: mark.offsetLeft, top: mark.offsetTop - el.scrollTop, y: mark.offsetTop + mark.offsetHeight - el.scrollTop };
  div.remove();
  return xy;
}

type Link = { number: number; title: string; state: string; pull: boolean };
type MentionProps = Omit<TextareaHTMLAttributes<HTMLTextAreaElement>, "value" | "onChange"> & { value: string; onValue: (v: string) => void; people: string[]; links?: Link[]; boxRef?: Ref<HTMLTextAreaElement> };
type Option = { key: string; insert: string; node: ReactNode };

const linkColor: Record<string, string> = { OPEN: "text-green-600", MERGED: "text-purple-600", CLOSED: "text-red-600" };

const fold = (t: string) => t.normalize("NFD").replace(/[\u0300-\u036f]/g, "").toLowerCase();

// rank orders issues and pull requests by how well their title matches free
// text: each word of the query found in the title (as a word start or inside a
// word) adds to the score. A number matches by its digits.
function rank(query: string, links: Link[]): Link[] {
  const q = fold(query).trim();
  if (!q) return links;
  if (/^\d+$/.test(q)) return links.filter((l) => String(l.number).startsWith(q));
  const words = q.split(/\s+/);
  return links
    .map((l) => {
      const title = fold(l.title), parts = title.split(/[^a-z0-9]+/);
      let score = 0;
      for (const w of words) score += parts.some((p) => p.startsWith(w)) ? 2 : title.includes(w) ? 1 : 0;
      return { l, score };
    })
    .filter((x) => x.score > 0)
    .sort((a, b) => b.score - a.score || b.l.number - a.l.number)
    .map((x) => x.l);
}

// options lists what "@" (people) or "#" (issues and pull requests) can complete.
function options(mark: string, query: string, people: string[], links: Link[]): Option[] {
  const q = query.toLowerCase();
  if (mark === "@") return people.filter((p) => p.toLowerCase().startsWith(q)).slice(0, 6).map((p) => ({
    key: p, insert: `@${p}`,
    node: <><img src={avatarUrl(p)} alt="" className="size-5 rounded-full bg-slate-200 dark:bg-hover" />{p}</>,
  }));
  return rank(query, links).slice(0, 8).map((l) => ({
    key: String(l.number), insert: `#${l.number}`,
    node: <>
      <span className={linkColor[l.state] ?? "text-slate-500"}><Icon i={l.pull ? icons.merge : icons.issue} className="size-4" /></span>
      <span className="font-mono text-xs text-slate-400">#{l.number}</span>
      <span className="min-w-0 flex-1 truncate">{l.title}</span>
    </>,
  }));
}

// MentionArea is a textarea that suggests people after "@", in a list that
// opens next to the caret. Arrows move, Enter or Tab pick, Escape closes.
export function MentionArea({ value, onValue, people, links = [], boxRef, onKeyDown, className, ...rest }: MentionProps) {
  const own = useRef<HTMLTextAreaElement | null>(null);
  const [m, setM] = useState<{ mark: string; start: number; query: string; x: number; y: number; top: number }>();
  const found = m ? options(m.mark, m.query, people, links) : [];
  const list = useRef<HTMLDivElement>(null);
  const [up, setUp] = useState(false);
  // Open the list above the caret when it would not fit below it.
  useLayoutEffect(() => {
    if (!m || !own.current || !list.current) return;
    const box = own.current.getBoundingClientRect(), h = list.current.offsetHeight + 4;
    setUp(box.top + m.y + h > window.innerHeight && box.top + m.top - h > 0);
  }, [m, found.length]);
  const [sel, setSel] = useState(0);
  const look = (el: HTMLTextAreaElement) => {
    const pos = el.selectionStart;
    const before = el.value.slice(0, pos);
    // "@" takes a login; "#" takes a number or free text up to the line end.
    const hit = /(^|[^\w`&])(@)([\w-]*)$/.exec(before) ?? /(^|[^\w`&])(#)([^\n#@]{0,40})$/.exec(before);
    // A finished reference ("#59 …") or a markdown heading ("# …") is not a search.
    if (!hit || (hit[2] === "#" && /^(\s|\d+\s)/.test(hit[3]))) return setM(undefined);
    const start = pos - hit[3].length - 1;
    setM({ mark: hit[2], start, query: hit[3], ...caretXY(el, start) });
    setSel(0);
  };
  const pick = (o: Option) => {
    const el = own.current;
    if (!el || !m) return;
    const end = m.start + 1 + m.query.length;
    const next = `${value.slice(0, m.start)}${o.insert} ${value.slice(end)}`;
    onValue(next);
    setM(undefined);
    const caret = m.start + o.insert.length + 1;
    requestAnimationFrame(() => { el.focus(); el.setSelectionRange(caret, caret); });
  };
  const setRef = (el: HTMLTextAreaElement | null) => {
    own.current = el;
    if (typeof boxRef === "function") boxRef(el);
    else if (boxRef) (boxRef as MutableRefObject<HTMLTextAreaElement | null>).current = el;
  };
  return (
    <div className="relative">
      <Textarea
        {...rest}
        ref={setRef}
        value={value}
        className={className}
        onChange={(e) => { onValue(e.target.value); look(e.target); }}
        onClick={(e) => look(e.currentTarget)}
        onBlur={() => setTimeout(() => setM(undefined), 120)}
        onKeyDown={(e) => {
          if (found.length) {
            if (e.key === "ArrowDown" || e.key === "ArrowUp") { e.preventDefault(); setSel((sel + (e.key === "ArrowDown" ? 1 : found.length - 1)) % found.length); return; }
            if (e.key === "Enter" || e.key === "Tab") { e.preventDefault(); pick(found[sel]); return; }
            if (e.key === "Escape") { e.preventDefault(); setM(undefined); return; }
          }
          onKeyDown?.(e);
        }}
      />
      {m && found.length > 0 && (
        <div ref={list} role="listbox" style={{ left: Math.max(0, Math.min(m.x, (own.current?.clientWidth ?? 400) - (m.mark === "#" ? 352 : 224))), ...(up ? { top: m.top - 4, transform: "translateY(-100%)" } : { top: m.y + 4 }) }} className={`absolute z-30 ${m.mark === "#" ? "w-88" : "w-56"} overflow-hidden rounded-xl border border-slate-200 bg-white p-1 shadow-xl dark:border-border dark:bg-surface`}>
          {found.map((p, i) => (
            <button key={p.key} type="button" role="option" aria-selected={i === sel} onMouseDown={(e) => { e.preventDefault(); pick(p); }} onMouseEnter={() => setSel(i)} className={`flex w-full items-center gap-2 rounded-lg px-2 py-1.5 text-left text-sm ${i === sel ? "bg-blue-50 text-blue-700 dark:bg-accent-subtle dark:text-accent-text" : "text-slate-700 dark:text-text"}`}>
              {p.node}
            </button>
          ))}
        </div>
      )}
    </div>
  );
}

// RefContext carries the repository of the view and its known issues and pull
// requests, so markdown can turn "#52" into a link to it.
export const RefContext = createContext<{ owner: string; repo: string; links: Link[] } | null>(null);

// refLinks rewrites "#52" outside code into a markdown link with the dory-ref scheme.
const refLinks = (md: string) =>
  md.split(/(```[\s\S]*?```|`[^`\n]*`)/).map((part, i) => (i % 2 ? part : part.replace(/(^|[^\w&/#[])#(\d+)\b/g, "$1[#$2](dory-ref:$2)"))).join("");

const refCls = "inline-flex items-center gap-0.5 rounded px-1 font-medium text-blue-700 no-underline hover:bg-blue-50 hover:underline dark:text-accent-text dark:hover:bg-accent-subtle";

// PrRefChip is the "#N" chip of a pull request: a state dot and the number,
// with a tooltip of its kind, title and state. Messages and the stack bar share it.
export function PrRefChip({ owner, repo, number, title, state, kind = "pulls", className, current }: { owner: string; repo: string; number: number; title: string; state: string; kind?: "pulls" | "issues"; className?: string; current?: boolean }) {
  const tip = `${kind === "pulls" ? "PR" : "Issue"} #${number} · ${title} · ${state === "OPEN" ? "abierta" : state === "MERGED" ? "mergeada" : "cerrada"}`;
  return (
    <Tip text={tip}>
      <RouterLink to="/github/$owner/$repo/$kind/$number" params={{ owner, repo, kind, number: String(number) }} aria-current={current ? "page" : undefined} className={`${refCls} ${className ?? ""}`}>
        <span className={`size-1.5 rounded-full ${state === "OPEN" ? "bg-green-500" : state === "MERGED" ? "bg-purple-500" : "bg-red-500"}`} />#{number}
      </RouterLink>
    </Tip>
  );
}

function RefLink({ number }: { number: number }) {
  const ctx = useContext(RefContext)!;
  const l = ctx.links.find((x) => x.number === number);
  if (!l) return <Tip text="Abrir en GitHub"><a href={`https://github.com/${ctx.owner}/${ctx.repo}/issues/${number}`} target="_blank" rel="noreferrer" className={refCls}>#{number}</a></Tip>;
  return <PrRefChip owner={ctx.owner} repo={ctx.repo} number={number} title={l.title} state={l.state} kind={l.pull ? "pulls" : "issues"} />;
}

// RefMd renders markdown where "#52" links to that issue or pull request.
export function RefMd({ children, onToggle }: { children: string; onToggle?: (line: number) => void }) {
  const ctx = useContext(RefContext);
  if (!ctx) return <Md onToggle={onToggle}>{children}</Md>;
  return (
    <Md onToggle={onToggle} urlTransform={(url) => (url.startsWith("dory-ref:") ? url : defaultUrlTransform(url))}
      components={{ a: ({ href, children: text, node: _, ...rest }) => (href?.startsWith("dory-ref:") ? <RefLink number={Number(href.slice(9))} /> : <a href={href} target="_blank" rel="noreferrer" {...rest}>{text}</a>) }}>
      {refLinks(children)}
    </Md>
  );
}
