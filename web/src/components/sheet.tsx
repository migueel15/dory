import { useEffect,type ReactNode } from "react";
import { createPortal } from "react-dom";

// Sheet is how every selector opens on phones: a panel that slides up from
// the bottom over a dimmed page, full width, never taller than 75% of the
// screen and scrolled inside. A tap on the backdrop closes it. It lives on
// document.body so no transformed or clipped parent can cut it.
export function Sheet({ title, onClose, children }: { title: string; onClose: () => void; children: ReactNode }) {
  useEffect(() => {
    const onKey = (e: KeyboardEvent) => { if (e.key === "Escape") onClose(); };
    document.addEventListener("keydown", onKey);
    return () => document.removeEventListener("keydown", onKey);
  }, [onClose]);
  return createPortal(
    <div data-sheet className="fixed inset-0 z-[70] flex flex-col justify-end">
      <div aria-hidden="true" onClick={onClose} className="absolute inset-0 animate-[sheet-fade_160ms_ease-out] bg-black/40" />
      <div role="dialog" aria-label={title}
        className="relative flex max-h-[75dvh] animate-[sheet-up_200ms_ease-out] flex-col rounded-t-3xl border-t border-slate-200 bg-white pb-[env(safe-area-inset-bottom)] shadow-2xl dark:border-border dark:bg-surface">
        <div aria-hidden="true" className="mx-auto mt-2.5 h-1 w-10 shrink-0 rounded-full bg-slate-300 dark:bg-border" />
        <h2 className="shrink-0 px-5 pb-2 pt-3 text-sm font-semibold text-slate-500 dark:text-muted">{title}</h2>
        <div className="min-h-0 flex-1 overflow-y-auto overscroll-contain px-2 pb-3">{children}</div>
      </div>
    </div>,
    document.body,
  );
}
