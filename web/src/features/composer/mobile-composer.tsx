import { Plus } from "@phosphor-icons/react";
import { useState,type FormHTMLAttributes,type ReactNode } from "react";
import { composerFrame } from "./composer-styles";
import { useOutside } from "./model-picker";

export const mobileSendButton = "grid size-9 shrink-0 place-items-center rounded-full bg-slate-900 text-white transition-colors disabled:bg-slate-200 disabled:text-slate-400 dark:bg-white dark:text-slate-900 dark:disabled:bg-raised dark:disabled:text-muted";
const plusButton = "[-webkit-tap-highlight-color:transparent] grid size-9 shrink-0 place-items-center rounded-full bg-slate-100 text-slate-600 dark:bg-raised dark:text-text-secondary";

// MobileComposer is the phone message box shared by the start screen and the
// threads: one line pinned to the bottom with +, the field and the actions.
// The + opens the extras floating above it, so nothing else moves. While
// writing or dictating, the field takes its own full-width line above the
// buttons and grows upward; the same node moves, so focus and keyboard stay.
export function MobileComposer({ form, extras, top, field, actions, below, active }: {
  form: FormHTMLAttributes<HTMLFormElement> & Record<`data-${string}`, unknown>;
  extras: (close: () => void) => ReactNode;
  top?: ReactNode;
  field: ReactNode;
  actions: ReactNode;
  below?: ReactNode;
  // active keeps the field on its own line without focus: a draft or a dictation.
  active?: boolean;
}) {
  const [open, setOpen] = useState(false);
  const [focused, setFocused] = useState(false);
  // Like a pop-up: a tap anywhere else closes the extras and folds the field.
  // Folding on blur instead would move the buttons under a finger mid-tap.
  const box = useOutside(open || focused, () => { setOpen(false); setFocused(false); });
  const expanded = focused || !!active;
  return (
    <div ref={box} className="relative">
      {open && (
        <div className="absolute inset-x-0 bottom-full z-20 mb-2 flex origin-bottom animate-[extras-in_160ms_ease-out] flex-wrap items-center gap-1 rounded-2xl border border-slate-200 bg-white p-2 text-xs text-slate-500 shadow-sm dark:border-border dark:bg-surface dark:text-muted">
          {extras(() => setOpen(false))}
        </div>
      )}
      <form {...form} className={composerFrame}>
        {top}
        <div className="flex flex-wrap items-end gap-x-0.5 gap-y-1 p-1.5">
          <button type="button" aria-label="Más opciones" aria-expanded={open} onClick={() => setOpen(!open)} className={plusButton}>
            <Plus weight="regular" className={`size-5 transition-transform duration-200 ${open ? "rotate-45" : ""}`} />
          </button>
          <div className={`relative min-w-0 ${expanded ? "order-first basis-full px-2" : "flex-1"}`} onFocus={() => { setOpen(false); setFocused(true); }}>{field}</div>
          <div className="ml-auto flex items-end gap-0.5">{actions}</div>
        </div>
        {below}
      </form>
    </div>
  );
}
