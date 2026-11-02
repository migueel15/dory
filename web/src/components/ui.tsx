import { useEffect, useLayoutEffect, useRef, useState } from "react";
import { createPortal } from "react-dom";
import type { ButtonHTMLAttributes, CSSProperties, InputHTMLAttributes, KeyboardEvent, ReactNode, TextareaHTMLAttributes } from "react";
import Markdown, { defaultUrlTransform, type Options } from "react-markdown";
import remarkGfm from "remark-gfm";
export { defaultUrlTransform };

const cx = (...c: (string | false | undefined)[]) => c.filter(Boolean).join(" ");

export function Button({ variant = "default", className, ...p }: ButtonHTMLAttributes<HTMLButtonElement> & { variant?: "default" | "ghost" | "danger" }) {
  const v = {
    default: "bg-slate-900 dark:bg-slate-100 text-white dark:text-slate-900 hover:bg-slate-700 dark:hover:bg-slate-300",
    ghost: "border border-slate-300 dark:border-slate-600 hover:bg-slate-100 dark:hover:bg-slate-800",
    danger: "border border-red-300 dark:border-red-800 text-red-700 dark:text-red-400 hover:bg-red-50 dark:hover:bg-red-950",
  }[variant];
  return <button {...p} className={cx("min-h-11 sm:min-h-0 rounded-md px-3 py-1.5 text-sm font-medium disabled:opacity-50", v, className)} />;
}

// pill styles links that act as secondary buttons (navigation, external links).
export const pill = "inline-flex min-h-11 sm:min-h-0 shrink-0 items-center gap-1.5 rounded-full border border-slate-200 bg-white px-3 py-1 text-xs font-medium text-slate-600 shadow-sm transition-colors hover:border-slate-300 hover:bg-slate-50 hover:text-slate-900 dark:border-slate-700 dark:bg-slate-900 dark:text-slate-300 dark:hover:border-slate-600 dark:hover:bg-slate-800 dark:hover:text-white";
export const Arrow = ({ back }: { back?: boolean }) => (
  <svg viewBox="0 0 20 20" fill="currentColor" className={cx("size-3.5", back && "rotate-180")}><path fillRule="evenodd" d="M3 10a.75.75 0 0 1 .75-.75h10.64l-4.22-4.22a.75.75 0 1 1 1.06-1.06l5.5 5.5a.75.75 0 0 1 0 1.06l-5.5 5.5a.75.75 0 1 1-1.06-1.06l4.22-4.22H3.75A.75.75 0 0 1 3 10Z" clipRule="evenodd" /></svg>
);

export const field = "min-h-11 sm:min-h-0 w-full rounded-md border border-slate-300 dark:border-slate-600 bg-white dark:bg-slate-900 px-2 py-1.5 text-sm";
export const Input = (p: InputHTMLAttributes<HTMLInputElement>) => <input {...p} className={cx(field, p.className)} />;
export const Textarea = (p: TextareaHTMLAttributes<HTMLTextAreaElement>) => <textarea {...p} className={cx(field, "min-h-24", p.className)} />;

export type Option = { value: string; label: string; group?: string };

// Select replaces the native <select>: a button that opens a floating, keyboard-navigable list.
export function Select({ value, onChange, options, disabled, className, placeholder = "—" }: { value: string; onChange: (v: string) => void; options: Option[]; disabled?: boolean; className?: string; placeholder?: string }) {
  const [open, setOpen] = useState(false);
  const [pos, setPos] = useState<CSSProperties>({});
  const [hi, setHi] = useState(0);
  const box = useRef<HTMLDivElement>(null);
  const list = useRef<HTMLDivElement>(null);
  const current = options.find((o) => o.value === value);
  // The list lives in a portal with fixed position, so modals and cards never clip it.
  useLayoutEffect(() => {
    if (!open) return;
    const place = () => {
      if (!box.current || !list.current) return;
      const t = box.current.getBoundingClientRect(), h = list.current.offsetHeight + 6;
      const up = t.bottom + h > window.innerHeight && t.top - h > 0;
      setPos({ left: t.left, width: t.width, ...(up ? { bottom: window.innerHeight - t.top + 6 } : { top: t.bottom + 6 }) });
    };
    place();
    window.addEventListener("resize", place);
    window.addEventListener("scroll", place, true);
    return () => { window.removeEventListener("resize", place); window.removeEventListener("scroll", place, true); };
  }, [open]);
  useLayoutEffect(() => {
    if (open) list.current?.querySelector<HTMLElement>("[aria-selected=true]")?.scrollIntoView({ block: "nearest" });
  }, [open, hi]);
  useEffect(() => {
    if (!open) return;
    const away = (e: MouseEvent) => !box.current?.contains(e.target as Node) && !list.current?.contains(e.target as Node) && setOpen(false);
    document.addEventListener("mousedown", away);
    return () => document.removeEventListener("mousedown", away);
  }, [open]);
  const show = () => { setHi(Math.max(0, options.findIndex((o) => o.value === value))); setOpen(true); };
  const pick = (o: Option) => { onChange(o.value); setOpen(false); };
  const key = (e: KeyboardEvent) => {
    if (!open) { if (["ArrowDown", "ArrowUp", "Enter", " "].includes(e.key)) { e.preventDefault(); show(); } return; }
    if (e.key === "Escape" || e.key === "Tab") { if (e.key === "Escape") e.preventDefault(); setOpen(false); }
    else if (e.key === "ArrowDown") { e.preventDefault(); setHi((i) => Math.min(options.length - 1, i + 1)); }
    else if (e.key === "ArrowUp") { e.preventDefault(); setHi((i) => Math.max(0, i - 1)); }
    else if (e.key === "Enter" || e.key === " ") { e.preventDefault(); if (options[hi]) pick(options[hi]); }
  };
  return (
    <div ref={box} className={cx("relative", className ?? "w-full")}>
      <button type="button" disabled={disabled} aria-haspopup="listbox" aria-expanded={open} onClick={() => (open ? setOpen(false) : show())} onKeyDown={key}
        className={cx(field, "flex items-center justify-between gap-2 text-left transition-colors hover:border-slate-400 focus:outline-none focus-visible:ring-2 focus-visible:ring-blue-500/40 disabled:opacity-50 dark:hover:border-slate-500", open && "border-blue-500 dark:border-blue-400")}>
        <span className={cx("truncate", !current && "text-slate-400")}>{current?.label ?? placeholder}</span>
        <svg viewBox="0 0 20 20" fill="currentColor" className={cx("size-4 shrink-0 text-slate-400 transition-transform", open && "rotate-180")}><path fillRule="evenodd" d="M5.23 7.21a.75.75 0 0 1 1.06.02L10 11.17l3.71-3.94a.75.75 0 1 1 1.08 1.04l-4.25 4.5a.75.75 0 0 1-1.08 0l-4.25-4.5a.75.75 0 0 1 .02-1.06Z" clipRule="evenodd" /></svg>
      </button>
      {open && createPortal(
        <div ref={list} role="listbox" style={pos} className="fixed z-50 max-h-72 min-w-48 overflow-auto rounded-xl border border-slate-200 bg-white p-1 shadow-xl dark:border-slate-700 dark:bg-slate-900">
          {options.length === 0 && <p className="px-2.5 py-2 text-sm text-slate-400">Sin opciones</p>}
          {options.map((o, i) => (
            <div key={o.value}>
              {o.group && o.group !== options[i - 1]?.group && <p className="px-2.5 pb-1 pt-2 text-[11px] font-semibold uppercase tracking-wide text-slate-400">{o.group}</p>}
              <button type="button" role="option" aria-selected={i === hi} onMouseEnter={() => setHi(i)} onMouseDown={(e) => e.preventDefault()} onClick={() => pick(o)}
                className={cx("flex w-full items-center justify-between gap-2 rounded-lg px-2.5 py-2 text-left text-sm", i === hi ? "bg-slate-100 dark:bg-slate-800" : "", o.value === value ? "font-medium text-blue-700 dark:text-blue-300" : "text-slate-700 dark:text-slate-200")}>
                <span className="truncate">{o.label}</span>
                {o.value === value && <svg viewBox="0 0 20 20" fill="currentColor" className="size-4 shrink-0"><path fillRule="evenodd" d="M16.7 5.3a1 1 0 0 1 0 1.4l-8 8a1 1 0 0 1-1.4 0l-4-4a1 1 0 1 1 1.4-1.4L8 12.58l7.3-7.3a1 1 0 0 1 1.4 0Z" clipRule="evenodd" /></svg>}
              </button>
            </div>
          ))}
        </div>,
        document.body,
      )}
    </div>
  );
}

export const opts = (vs: readonly string[]): Option[] => vs.map((v) => ({ value: v, label: v }));

export const Card = ({ children, className }: { children: ReactNode; className?: string }) => (
  <div className={cx("rounded-lg border border-slate-200 dark:border-slate-700 bg-white dark:bg-slate-900 p-3 shadow-sm", className)}>{children}</div>
);

export const Label = ({ text, children }: { text: string; children: ReactNode }) => (
  <label className="block space-y-1 text-sm">
    <span className="text-slate-600 dark:text-slate-300">{text}</span>
    {children}
  </label>
);

export const Badge = ({ children }: { children: ReactNode }) => (
  <span className="rounded bg-slate-100 dark:bg-slate-800 px-1.5 py-0.5 text-xs text-slate-700 dark:text-slate-200">{children}</span>
);

// toggleTask flips the checkbox of a markdown task item on the given 1-based line.
export function toggleTask(md: string, line: number) {
  const lines = md.split("\n");
  lines[line - 1] = lines[line - 1]?.replace(/^(\s*(?:>\s*)*(?:[-*+]|\d+[.)])\s+)\[([ xX])\]/, (_, lead: string, mark: string) => `${lead}[${mark === " " ? "x" : " "}]`);
  return lines.join("\n");
}

const prose = [
  "space-y-3 break-words text-sm leading-relaxed text-slate-700 dark:text-slate-200",
  "[&_h1]:border-b [&_h1]:border-slate-200 [&_h1]:pb-1 [&_h1]:text-xl [&_h1]:font-bold dark:[&_h1]:border-slate-700",
  "[&_h2]:border-b [&_h2]:border-slate-200 [&_h2]:pb-1 [&_h2]:text-lg [&_h2]:font-semibold dark:[&_h2]:border-slate-700",
  "[&_h3]:text-base [&_h3]:font-semibold [&_h4]:font-semibold [&_h1,&_h2,&_h3,&_h4]:mt-4 [&_h1,&_h2,&_h3,&_h4]:text-slate-900 dark:[&_h1,&_h2,&_h3,&_h4]:text-white",
  "[&_a]:text-blue-600 [&_a]:underline-offset-2 hover:[&_a]:underline dark:[&_a]:text-blue-400",
  "[&_ul]:ml-5 [&_ul]:list-disc [&_ol]:ml-5 [&_ol]:list-decimal [&_li]:my-0.5 [&_li>ul]:mt-1 [&_li>ol]:mt-1",
  "[&_ul.contains-task-list]:ml-0 [&_ul.contains-task-list]:list-none [&_li.task-list-item]:flex [&_li.task-list-item]:flex-wrap [&_li.task-list-item]:items-baseline [&_li.task-list-item]:gap-2 [&_li.task-list-item>ul]:basis-full [&_li.task-list-item>ul]:ml-6",
  "[&_input[type=checkbox]]:size-4 [&_input[type=checkbox]]:translate-y-0.5 [&_input[type=checkbox]]:accent-green-600",
  "[&_:not(pre)>code]:rounded [&_:not(pre)>code]:bg-slate-100 [&_:not(pre)>code]:px-1.5 [&_:not(pre)>code]:py-0.5 [&_:not(pre)>code]:font-mono [&_:not(pre)>code]:text-[0.85em] dark:[&_:not(pre)>code]:bg-slate-800",
  "[&_pre]:overflow-auto [&_pre]:rounded-lg [&_pre]:border [&_pre]:border-slate-200 [&_pre]:bg-slate-50 [&_pre]:p-3 [&_pre]:font-mono [&_pre]:text-xs dark:[&_pre]:border-slate-700 dark:[&_pre]:bg-slate-950",
  "[&_blockquote]:rounded-r [&_blockquote]:border-l-4 [&_blockquote]:border-slate-300 [&_blockquote]:bg-slate-50 [&_blockquote]:px-3 [&_blockquote]:py-1 [&_blockquote]:text-slate-600 dark:[&_blockquote]:border-slate-600 dark:[&_blockquote]:bg-slate-800/50 dark:[&_blockquote]:text-slate-300",
  "[&_table]:block [&_table]:max-w-full [&_table]:overflow-auto [&_table]:border-collapse [&_th]:border [&_th]:border-slate-200 [&_th]:bg-slate-50 [&_th]:px-3 [&_th]:py-1.5 [&_th]:text-left [&_th]:font-semibold [&_td]:border [&_td]:border-slate-200 [&_td]:px-3 [&_td]:py-1.5 dark:[&_th]:border-slate-700 dark:[&_th]:bg-slate-800 dark:[&_td]:border-slate-700 [&_tr:nth-child(even)_td]:bg-slate-50/60 dark:[&_tr:nth-child(even)_td]:bg-slate-800/30",
  "[&_hr]:my-4 [&_hr]:border-slate-200 dark:[&_hr]:border-slate-700 [&_img]:max-w-full [&_img]:rounded-md [&_del]:text-slate-400",
].join(" ");

// Md renders GitHub-flavored markdown. With onToggle, task checkboxes can be
// clicked and report the source line of their item.
export function Md({ children, components, urlTransform, onToggle }: { children: string; onToggle?: (line: number) => void } & Pick<Options, "components" | "urlTransform">) {
  const tasks: Options["components"] = onToggle && {
    li: ({ node, children: content, ...rest }) => {
      const line = node?.position?.start.line;
      return <li {...rest} onClick={(e) => { if (line && (e.target as HTMLElement).matches("input[type=checkbox]") && e.target === (e.currentTarget as HTMLElement).querySelector(":scope > input[type=checkbox]")) onToggle(line); }}>{content}</li>;
    },
    input: ({ node: _, disabled: __, ...rest }) => <input {...rest} readOnly className="cursor-pointer" />,
  };
  return (
    <div className={prose}>
      <Markdown remarkPlugins={[remarkGfm]} components={{ ...tasks, ...components }} urlTransform={urlTransform}>{children}</Markdown>
    </div>
  );
}

export function Modal({ title, onClose, children }: { title: string; onClose: () => void; children: ReactNode }) {
  return (
    <div className="fixed inset-0 z-10 flex items-center justify-center bg-black/40 p-2 sm:p-4" onClick={onClose}>
      <div className="max-h-full w-full max-w-lg space-y-3 overflow-auto rounded-lg bg-white dark:bg-slate-900 p-3 sm:p-4" onClick={(e) => e.stopPropagation()}>
        <h2 className="text-lg font-semibold">{title}</h2>
        {children}
      </div>
    </div>
  );
}

export const csv = (s: string) => s.split(",").map((t) => t.trim()).filter(Boolean);
export const errMsg = (e: unknown) => (e instanceof Error ? e.message : "");
export const ErrorText = ({ error }: { error: unknown }) => (error ? <p className="text-sm text-red-600 dark:text-red-400">{errMsg(error)}</p> : null);
