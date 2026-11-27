import { useEffect, useLayoutEffect, useRef, useState } from "react";
import { ArrowRight, CaretUpDown, Check, MagnifyingGlass, X } from "@phosphor-icons/react";
import { createPortal } from "react-dom";
import type { ButtonHTMLAttributes, CSSProperties, InputHTMLAttributes, KeyboardEvent, ReactNode, Ref, TextareaHTMLAttributes } from "react";
import { errorText } from "../lib/errors";

export const iconButton = "inline-flex h-8 min-w-8 shrink-0 items-center justify-center rounded-md text-slate-500 enabled:hover:bg-slate-100 enabled:hover:text-slate-900 disabled:opacity-40 dark:text-text-secondary dark:enabled:hover:bg-raised dark:enabled:hover:text-white";

const cx = (...c: (string | false | undefined)[]) => c.filter(Boolean).join(" ");

export function Button({ variant = "default", className, ...p }: ButtonHTMLAttributes<HTMLButtonElement> & { ref?: Ref<HTMLButtonElement>; variant?: "default" | "ghost" | "bare" | "danger" }) {
  const v = {
    default: "bg-slate-900 dark:bg-accent text-white dark:text-white hover:bg-slate-700 dark:hover:bg-accent-hover",
    ghost: "border border-slate-300 dark:border-border-strong hover:bg-slate-100 dark:hover:bg-raised",
    bare: "hover:bg-slate-200 dark:hover:bg-raised",
    danger: "border border-red-300 dark:border-red-800 text-red-700 dark:text-red-400 hover:bg-red-50 dark:hover:bg-red-950",
  }[variant];
  return <button {...p} className={cx("inline-flex min-h-11 items-center justify-center gap-1.5 whitespace-nowrap sm:min-h-0 rounded-md px-3 py-1.5 text-sm font-medium disabled:opacity-50", v, className)} />;
}

// pill styles links that act as secondary buttons (navigation, external links).
export const pill = "inline-flex min-h-11 sm:min-h-0 shrink-0 items-center gap-1.5 rounded-md border border-slate-200 bg-white px-3 py-1 text-xs font-medium text-slate-600 shadow-sm transition-colors hover:border-slate-300 hover:bg-slate-50 hover:text-slate-900 dark:border-border dark:bg-surface dark:text-text-secondary dark:hover:border-border-strong dark:hover:bg-raised dark:hover:text-white";
export const Arrow = ({ back }: { back?: boolean }) => (
  <ArrowRight weight="regular" aria-hidden="true" className={cx("size-3.5", back && "rotate-180")} />
);

// keepAboveKeyboard scrolls box, smoothly, until its bottom sits just above the phone keyboard.
// It measures again on every step of the keyboard animation, so it never overshoots.
export function keepAboveKeyboard(box: HTMLElement | null | undefined, gap = 16) {
  const vv = window.visualViewport;
  if (!vv || !box) return;
  const fit = () => {
    const bottom = box.getBoundingClientRect().bottom, visible = vv.offsetTop + vv.height;
    if (Math.abs(bottom - (visible - gap)) < 2 || bottom <= visible - gap) return;
    let el = box.parentElement;
    while (el && !(el.scrollHeight > el.clientHeight && /auto|scroll/.test(getComputedStyle(el).overflowY))) el = el.parentElement;
    (el ?? document.scrollingElement)?.scrollBy({ top: bottom - visible + gap, behavior: "smooth" });
  };
  vv.addEventListener("resize", fit);
  setTimeout(fit, 500);
  setTimeout(() => vv.removeEventListener("resize", fit), 1000);
}

// field is the one text-field style: neutral border that darkens on focus, never an accent outline.
export const field = "min-h-11 sm:min-h-0 w-full rounded-md border border-slate-300 dark:border-border-strong bg-white dark:bg-surface px-2 py-1.5 text-sm !outline-none transition-colors focus:border-slate-500 dark:focus:border-muted";
// bare drops border and background for fields embedded in a framed container (capture box, command palette).
const bareField = "w-full bg-transparent text-sm !outline-none";
export const Input = ({ bare, className, ...p }: InputHTMLAttributes<HTMLInputElement> & { bare?: boolean; ref?: Ref<HTMLInputElement> }) => <input {...p} className={cx(bare ? bareField : field, className)} />;
export const Textarea = ({ bare, className, ...p }: TextareaHTMLAttributes<HTMLTextAreaElement> & { bare?: boolean; ref?: Ref<HTMLTextAreaElement> }) => <textarea {...p} className={cx(bare ? bareField : field, !bare && !p.rows && "min-h-24", className)} />;

export type Option = { value: string; label: string; group?: string; icon?: ReactNode; disabled?: boolean };

// Select replaces the native <select>: a button that opens a floating, keyboard-navigable
// list. Long lists get a search box; groups, icons and disabled options are supported.
export function Select({ value, onChange, options, disabled, className, placeholder = "—", plain, borderless, buttonRef }: { buttonRef?: React.Ref<HTMLButtonElement>; value: string; onChange: (v: string) => void; options: Option[]; disabled?: boolean; className?: string; placeholder?: string; plain?: boolean; borderless?: boolean }) {
  const [open, setOpen] = useState(false);
  const [pos, setPos] = useState<CSSProperties>({});
  const [hi, setHi] = useState(0);
  const [query, setQuery] = useState("");
  const box = useRef<HTMLDivElement>(null);
  const list = useRef<HTMLDivElement>(null);
  const current = options.find((o) => o.value === value);
  const searchable = options.length > 7;
  const q = query.trim().toLowerCase();
  const shown = q ? options.filter((o) => o.label.toLowerCase().includes(q) || o.group?.toLowerCase().includes(q)) : options;
  // The list lives in a portal with fixed position, so modals and cards never clip it.
  useLayoutEffect(() => {
    if (!open) return;
    const place = () => {
      if (!box.current || !list.current) return;
      const t = box.current.getBoundingClientRect(), h = list.current.offsetHeight + 6;
      const up = t.bottom + h > window.innerHeight && t.top - h > 0;
      const width = Math.max(t.width, 208);
      const left = Math.max(8, Math.min(t.left, window.innerWidth - width - 8));
      setPos({ left, width, ...(up ? { bottom: window.innerHeight - t.top + 6 } : { top: t.bottom + 6 }) });
    };
    place();
    window.addEventListener("resize", place);
    window.addEventListener("scroll", place, true);
    return () => { window.removeEventListener("resize", place); window.removeEventListener("scroll", place, true); };
  }, [open]);
  useLayoutEffect(() => {
    if (open) list.current?.querySelector<HTMLElement>("[data-hi=true]")?.scrollIntoView({ block: "nearest" });
  }, [open, hi]);
  useEffect(() => {
    if (!open) { setQuery(""); return; }
    const away = (e: MouseEvent) => !box.current?.contains(e.target as Node) && !list.current?.contains(e.target as Node) && setOpen(false);
    document.addEventListener("mousedown", away);
    return () => document.removeEventListener("mousedown", away);
  }, [open]);
  const show = () => { setHi(Math.max(0, options.findIndex((o) => o.value === value))); setOpen(true); };
  const pick = (o: Option) => { onChange(o.value); setOpen(false); };
  const key = (e: KeyboardEvent) => {
    if (!open) { if (["ArrowDown", "ArrowUp", "Enter", " "].includes(e.key)) { e.preventDefault(); show(); } return; }
    // Escape closes only the list, not the modal that holds it.
    if (e.key === "Escape" || e.key === "Tab") { if (e.key === "Escape") { e.preventDefault(); e.nativeEvent.stopImmediatePropagation(); } setOpen(false); }
    else if (e.key === "ArrowDown") { e.preventDefault(); setHi((i) => Math.min(shown.length - 1, i + 1)); }
    else if (e.key === "ArrowUp") { e.preventDefault(); setHi((i) => Math.max(0, i - 1)); }
    else if (e.key === "Enter" || (e.key === " " && !searchable)) { e.preventDefault(); if (shown[hi] && !shown[hi].disabled) pick(shown[hi]); }
  };
  return (
    <div ref={box} className={cx("relative", className ?? "w-full")}>
      <button ref={buttonRef} type="button" disabled={disabled} aria-haspopup="listbox" aria-expanded={open} onClick={() => (open ? setOpen(false) : show())} onKeyDown={key}
        className={cx(
          borderless ? "group flex min-h-11 w-full items-center justify-between gap-2 rounded-sm bg-transparent py-1 text-left text-sm font-medium text-slate-600 transition-colors enabled:hover:text-slate-950 focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-accent disabled:opacity-50 dark:text-text-secondary dark:enabled:hover:text-white sm:min-h-8"
            : plain ? "flex min-h-11 items-center gap-1 rounded-md px-2 py-1 text-left text-sm hover:bg-slate-200 dark:hover:bg-raised sm:min-h-0"
            : "flex min-h-11 w-full items-center justify-between gap-2 rounded-lg border border-slate-300 bg-white px-2.5 py-1.5 text-left text-sm shadow-sm transition-colors hover:border-slate-400 hover:bg-slate-50 disabled:opacity-50 dark:border-border-strong dark:bg-surface dark:hover:border-muted dark:hover:bg-raised sm:min-h-0",
          !borderless && !plain && open && "border-slate-500 bg-slate-50 dark:border-muted dark:bg-raised")}>
        <span className={cx("flex min-w-0 items-center gap-2 truncate", !current && "text-slate-400 dark:text-muted")}>{current?.icon}{current?.label ?? placeholder}</span>
        <CaretUpDown weight="regular" aria-hidden="true" className={cx("size-4 shrink-0", borderless ? "opacity-70 transition-opacity group-hover:opacity-100" : "text-slate-400")} />
      </button>
      {open && createPortal(
        <div ref={list} style={pos} className="notif-in fixed z-[100] overflow-hidden rounded-lg border border-slate-200 bg-white shadow-xl dark:border-border dark:bg-surface">
          {searchable && (
            <div className="relative border-b border-slate-200 p-1.5 dark:border-border">
              <MagnifyingGlass weight="regular" aria-hidden="true" className="pointer-events-none absolute left-3.5 top-1/2 size-4 -translate-y-1/2 text-slate-400" />
              <Input bare autoFocus value={query} onChange={(e) => { setQuery(e.target.value); setHi(0); }} onKeyDown={key} placeholder="Buscar…" aria-label="Buscar opción" className="py-1.5 pl-8 pr-2" />
            </div>
          )}
          <div role="listbox" className="max-h-72 overflow-auto p-1">
            {shown.length === 0 && <p className="px-2.5 py-2 text-sm text-slate-400">{q ? "Nada coincide" : "Sin opciones"}</p>}
            {shown.map((o, i) => (
              <div key={o.value}>
                {o.group && o.group !== shown[i - 1]?.group && <p className="px-2.5 pb-1 pt-2 text-[0.6875rem] font-semibold uppercase tracking-wide text-slate-400 dark:text-muted">{o.group}</p>}
                <button type="button" role="option" data-hi={i === hi} aria-selected={o.value === value} aria-disabled={o.disabled} disabled={o.disabled} onMouseEnter={() => setHi(i)} onMouseDown={(e) => e.preventDefault()} onClick={() => pick(o)}
                  className={cx("flex min-h-11 w-full items-center gap-2 rounded-md px-2 py-1.5 text-left text-sm sm:min-h-0", i === hi ? "bg-slate-100 dark:bg-raised" : "", o.value === value ? "font-medium text-slate-900 dark:text-white" : "text-slate-700 dark:text-text-secondary", o.disabled && "cursor-not-allowed opacity-50")}>
                  <span className="grid size-4 shrink-0 place-items-center">{o.value === value && <Check aria-hidden="true" weight="bold" className="size-3.5" />}</span>
                  <span className="flex min-w-0 flex-1 items-center gap-2 truncate">{o.icon}{o.label}</span>
                </button>
              </div>
            ))}
          </div>
        </div>,
        // Keep the menu inside its dialog or popover so it appears in the same top layer.
        box.current?.closest("dialog, [popover]:popover-open") ?? document.body,
      )}
    </div>
  );
}

export const opts = (vs: readonly string[]): Option[] => vs.map((v) => ({ value: v, label: v }));

export const Card = ({ children, className }: { children: ReactNode; className?: string }) => (
  <div className={cx("rounded-lg border border-slate-200 dark:border-border bg-white dark:bg-surface p-3 shadow-sm", className)}>{children}</div>
);

export const Label = ({ text, children }: { text: string; children: ReactNode }) => (
  <label className="block space-y-1 text-sm">
    <span className="text-slate-600 dark:text-text-secondary">{text}</span>
    {children}
  </label>
);

export const Badge = ({ children }: { children: ReactNode }) => (
  <span className="rounded bg-slate-100 dark:bg-raised px-1.5 py-0.5 text-xs text-slate-700 dark:text-text">{children}</span>
);

// toggleTask flips the checkbox of a markdown task item on the given 1-based line.
export function toggleTask(md: string, line: number) {
  const lines = md.split("\n");
  lines[line - 1] = lines[line - 1]?.replace(/^(\s*(?:>\s*)*(?:[-*+]|\d+[.)])\s+)\[([ xX])\]/, (_, lead: string, mark: string) => `${lead}[${mark === " " ? "x" : " "}]`);
  return lines.join("\n");
}

export const prose = [
  "space-y-3 break-words text-sm leading-relaxed text-slate-700 dark:text-text",
  "[&_h1]:border-b [&_h1]:border-slate-200 [&_h1]:pb-1 [&_h1]:text-xl [&_h1]:font-bold dark:[&_h1]:border-border",
  "[&_h2]:border-b [&_h2]:border-slate-200 [&_h2]:pb-1 [&_h2]:text-lg [&_h2]:font-semibold dark:[&_h2]:border-border",
  "[&_h3]:text-base [&_h3]:font-semibold [&_h4]:font-semibold [&_h1,&_h2,&_h3,&_h4]:mt-4 [&_h1,&_h2,&_h3,&_h4]:text-slate-900 dark:[&_h1,&_h2,&_h3,&_h4]:text-white",
  "[&_a]:text-accent [&_a]:underline-offset-2 hover:[&_a]:underline dark:[&_a]:text-accent-text",
  "[&_ul]:ml-5 [&_ul]:list-disc [&_ol]:ml-5 [&_ol]:list-decimal [&_li]:my-0.5 [&_li>ul]:mt-1 [&_li>ol]:mt-1",
  "[&_ul.contains-task-list]:ml-0 [&_ul.contains-task-list]:list-none [&_li.task-list-item]:flex [&_li.task-list-item]:flex-wrap [&_li.task-list-item]:items-baseline [&_li.task-list-item]:gap-2 [&_li.task-list-item>ul]:basis-full [&_li.task-list-item>ul]:ml-6",
  "[&_input[type=checkbox]]:size-4 [&_input[type=checkbox]]:translate-y-0.5 [&_input[type=checkbox]]:accent-green-600",
  "[&_:not(pre)>code]:rounded [&_:not(pre)>code]:bg-slate-100 [&_:not(pre)>code]:px-1.5 [&_:not(pre)>code]:py-0.5 [&_:not(pre)>code]:font-mono [&_:not(pre)>code]:text-[0.85em] dark:[&_:not(pre)>code]:bg-raised",
  "[&_pre]:overflow-auto [&_pre]:rounded-lg [&_pre]:border [&_pre]:border-slate-200 [&_pre]:bg-slate-50 [&_pre]:p-3 [&_pre]:font-mono [&_pre]:text-xs dark:[&_pre]:border-border dark:[&_pre]:bg-canvas",
  "[&_blockquote]:rounded-r [&_blockquote]:border-l-4 [&_blockquote]:border-slate-300 [&_blockquote]:bg-slate-50 [&_blockquote]:px-3 [&_blockquote]:py-1 [&_blockquote]:text-slate-600 dark:[&_blockquote]:border-border-strong dark:[&_blockquote]:bg-raised/50 dark:[&_blockquote]:text-text-secondary",
  "[&_table]:block [&_table]:max-w-full [&_table]:overflow-auto [&_table]:border-collapse [&_th]:border [&_th]:border-slate-200 [&_th]:bg-slate-50 [&_th]:px-3 [&_th]:py-1.5 [&_th]:text-left [&_th]:font-semibold [&_td]:border [&_td]:border-slate-200 [&_td]:px-3 [&_td]:py-1.5 dark:[&_th]:border-border dark:[&_th]:bg-raised dark:[&_td]:border-border [&_tr:nth-child(even)_td]:bg-slate-50/60 dark:[&_tr:nth-child(even)_td]:bg-raised/30",
  "[&_hr]:my-4 [&_hr]:border-slate-200 dark:[&_hr]:border-border [&_img]:max-w-full [&_img]:rounded-md [&_del]:text-slate-400",
].join(" ");

export function Modal({ title, onClose, children, wide = false, className }: { title: string; onClose: () => void; children: ReactNode; wide?: boolean; className?: string }) {
  useEffect(() => {
    const onKey = (e: globalThis.KeyboardEvent) => { if (e.key === "Escape") { e.stopPropagation(); onClose(); } };
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [onClose]);
  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/40 p-2 sm:p-4" onClick={onClose}>
      <div role="dialog" aria-modal="true" aria-label={title} className={`max-h-full w-full max-w-lg space-y-3 overflow-auto rounded-lg bg-white dark:bg-surface p-3 sm:p-4${wide ? " lg:max-w-3xl" : ""} ${className ?? ""}`} onClick={(e) => e.stopPropagation()}>
        <div className="flex items-start justify-between gap-2">
          <h2 className="min-w-0 wrap-break-word text-lg font-semibold">{title}</h2>
          <button type="button" aria-label="Cerrar" onClick={onClose} className="-mr-1 grid size-8 shrink-0 place-items-center text-slate-500 hover:text-slate-900 dark:text-muted dark:hover:text-white">
            <X weight="regular" aria-hidden="true" className="size-4" />
          </button>
        </div>
        {children}
      </div>
    </div>
  );
}

// PageHeader gives every page the same title scale, optional subtitle and actions.
export const PageHeader = ({ title, subtitle, children }: { title: string; subtitle?: string; children?: ReactNode }) => (
  <div className="flex flex-wrap items-center justify-between gap-2">
    <div className="flex min-w-0 items-baseline gap-2">
      <h1 className="text-xl font-bold">{title}</h1>
      {subtitle && <span className="truncate text-sm text-slate-500 dark:text-muted">{subtitle}</span>}
    </div>
    {children && <div className="flex flex-wrap items-center gap-2">{children}</div>}
  </div>
);

// Notice covers the loading and empty states of lists so every page words them the same way.
export const Notice = ({ loading, children }: { loading?: boolean; children?: ReactNode }) => (
  <p className="rounded-lg border border-dashed border-slate-300 px-3 py-6 text-center text-sm text-slate-500 dark:border-border-strong dark:text-muted">{loading ? "Cargando…" : children}</p>
);

// Tones shared by every status pill (items, jobs, threads, project status).
export const TONES = {
  slate: "bg-slate-100 text-slate-700 dark:bg-raised dark:text-text-secondary",
  sky: "bg-sky-100 text-sky-800 dark:bg-sky-950 dark:text-sky-300",
  indigo: "bg-indigo-100 text-indigo-800 dark:bg-indigo-950 dark:text-indigo-300",
  amber: "bg-amber-100 text-amber-800 dark:bg-amber-950 dark:text-amber-300",
  violet: "bg-violet-100 text-violet-800 dark:bg-violet-950 dark:text-violet-300",
  red: "bg-red-100 text-red-800 dark:bg-red-950 dark:text-red-300",
  green: "bg-emerald-100 text-emerald-800 dark:bg-emerald-950 dark:text-emerald-300",
  blue: "bg-blue-100 text-blue-800 dark:bg-accent-subtle dark:text-accent-text",
} as const;
export type Tone = keyof typeof TONES;
export const StatusPill = ({ tone = "slate", children }: { tone?: keyof typeof TONES; children: ReactNode }) => (
  <span className={`rounded-full px-2.5 py-0.5 text-xs font-medium ${TONES[tone]}`}>{children}</span>
);

// uuid falls back to getRandomValues: crypto.randomUUID needs HTTPS or localhost.
export const uuid = () =>
  typeof crypto.randomUUID === "function"
    ? crypto.randomUUID()
    : "10000000-1000-4000-8000-100000000000".replace(/[018]/g, (c) => (+c ^ (crypto.getRandomValues(new Uint8Array(1))[0] & (15 >> (+c / 4)))).toString(16));

export const csv = (s: string) => s.split(",").map((t) => t.trim()).filter(Boolean);
export const errMsg = errorText;
export const ErrorText = ({ error }: { error: unknown }) => (error ? <p className="text-sm text-red-600 dark:text-red-400">{errMsg(error)}</p> : null);

// copyText copies to the clipboard. The Clipboard API exists only on HTTPS or
// localhost, so plain HTTP (the dev preview, LAN access) falls back to execCommand.
export async function copyText(text: string): Promise<boolean> {
  try {
    if (navigator.clipboard && window.isSecureContext) { await navigator.clipboard.writeText(text); return true; }
  } catch { /* fall back below */ }
  const area = document.createElement("textarea");
  area.value = text;
  area.setAttribute("readonly", "");
  area.style.cssText = "position:fixed;top:0;left:0;opacity:0;pointer-events:none";
  // Inside a modal dialog the textarea must live in it to be focusable.
  (document.activeElement?.closest("dialog") ?? document.body).appendChild(area);
  area.select();
  let ok = false;
  try { ok = document.execCommand("copy"); } catch { ok = false; }
  area.remove();
  return ok;
}
