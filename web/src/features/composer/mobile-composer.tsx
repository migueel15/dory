import { Plus } from "@phosphor-icons/react";
import { useState,type FormHTMLAttributes,type ReactNode } from "react";
import { composerFrame } from "./composer-styles";
import { useOutside } from "./model-picker";

export const mobileSendButton = "grid size-9 shrink-0 place-items-center rounded-full bg-slate-900 text-white transition-colors disabled:bg-slate-200 disabled:text-slate-400 dark:bg-white dark:text-slate-900 dark:disabled:bg-raised dark:disabled:text-muted";
const plusButton = "[-webkit-tap-highlight-color:transparent] grid size-9 shrink-0 place-items-center rounded-full bg-slate-100 text-slate-600 dark:bg-raised dark:text-text-secondary";

// MobileComposer is the phone message box shared by the start screen and the
// threads: one line pinned to the bottom with +, the field and the actions.
// The + opens the extras floating above it, so nothing else moves.
export function MobileComposer({ form, extras, top, field, actions, below }: {
  form: FormHTMLAttributes<HTMLFormElement> & Record<`data-${string}`, unknown>;
  extras: (close: () => void) => ReactNode;
  top?: ReactNode;
  field: ReactNode;
  actions: ReactNode;
  below?: ReactNode;
}) {
  const [open, setOpen] = useState(false);
  // Like a pop-up: a tap anywhere else closes the extras.
  const box = useOutside(open, () => setOpen(false));
  return (
    <div ref={box} className="relative">
      {open && (
        <div className="absolute inset-x-0 bottom-full z-20 mb-2 flex origin-bottom animate-[extras-in_160ms_ease-out] flex-wrap items-center gap-1 rounded-2xl border border-slate-200 bg-white p-2 text-xs text-slate-500 shadow-sm dark:border-border dark:bg-surface dark:text-muted">
          {extras(() => setOpen(false))}
        </div>
      )}
      <form {...form} className={composerFrame}>
        {top}
        <div className="flex items-end gap-0.5 p-1.5">
          <button type="button" aria-label="Más opciones" aria-expanded={open} onClick={() => setOpen(!open)} className={plusButton}>
            <Plus weight="regular" className={`size-5 transition-transform duration-200 ${open ? "rotate-45" : ""}`} />
          </button>
          <div className="relative min-w-0 flex-1" onFocus={() => setOpen(false)}>{field}</div>
          {actions}
        </div>
        {below}
      </form>
    </div>
  );
}
