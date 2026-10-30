import type { ButtonHTMLAttributes, InputHTMLAttributes, ReactNode, TextareaHTMLAttributes } from "react";
import Markdown from "react-markdown";

const cx = (...c: (string | false | undefined)[]) => c.filter(Boolean).join(" ");

export function Button({ variant = "default", className, ...p }: ButtonHTMLAttributes<HTMLButtonElement> & { variant?: "default" | "ghost" | "danger" }) {
  const v = {
    default: "bg-slate-900 text-white hover:bg-slate-700",
    ghost: "border border-slate-300 hover:bg-slate-100",
    danger: "border border-red-300 text-red-700 hover:bg-red-50",
  }[variant];
  return <button {...p} className={cx("min-h-11 sm:min-h-0 rounded-md px-3 py-1.5 text-sm font-medium disabled:opacity-50", v, className)} />;
}

export const field = "min-h-11 sm:min-h-0 w-full rounded-md border border-slate-300 bg-white px-2 py-1.5 text-sm";
export const Input = (p: InputHTMLAttributes<HTMLInputElement>) => <input {...p} className={cx(field, p.className)} />;
export const Textarea = (p: TextareaHTMLAttributes<HTMLTextAreaElement>) => <textarea {...p} className={cx(field, "min-h-24", p.className)} />;

export const Card = ({ children, className }: { children: ReactNode; className?: string }) => (
  <div className={cx("rounded-lg border border-slate-200 bg-white p-3 shadow-sm", className)}>{children}</div>
);

export const Label = ({ text, children }: { text: string; children: ReactNode }) => (
  <label className="block space-y-1 text-sm">
    <span className="text-slate-600">{text}</span>
    {children}
  </label>
);

export const Badge = ({ children }: { children: ReactNode }) => (
  <span className="rounded bg-slate-100 px-1.5 py-0.5 text-xs text-slate-700">{children}</span>
);

export const Md = ({ children }: { children: string }) => (
  <div className="prose-sm space-y-2 break-words text-sm [&_a]:text-blue-600 [&_code]:rounded [&_code]:bg-slate-100 [&_code]:px-1 [&_h1]:text-lg [&_h1]:font-bold [&_h2]:font-semibold [&_li]:ml-4 [&_li]:list-disc [&_pre]:overflow-auto [&_pre]:bg-slate-100 [&_pre]:p-2">
    <Markdown>{children}</Markdown>
  </div>
);

export function Modal({ title, onClose, children }: { title: string; onClose: () => void; children: ReactNode }) {
  return (
    <div className="fixed inset-0 z-10 flex items-center justify-center bg-black/40 p-2 sm:p-4" onClick={onClose}>
      <div className="max-h-full w-full max-w-lg space-y-3 overflow-auto rounded-lg bg-white p-3 sm:p-4" onClick={(e) => e.stopPropagation()}>
        <h2 className="text-lg font-semibold">{title}</h2>
        {children}
      </div>
    </div>
  );
}

export const csv = (s: string) => s.split(",").map((t) => t.trim()).filter(Boolean);
export const errMsg = (e: unknown) => (e instanceof Error ? e.message : "");
export const ErrorText = ({ error }: { error: unknown }) => (error ? <p className="text-sm text-red-600">{errMsg(error)}</p> : null);
