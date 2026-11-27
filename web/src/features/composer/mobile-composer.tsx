import { Plus } from "@phosphor-icons/react";
import { useLayoutEffect,useRef,useState,type FormHTMLAttributes,type ReactNode } from "react";
import { composerFrame } from "./composer-styles";
import { useOutside } from "./model-picker";

export const mobileSendButton = "grid size-9 shrink-0 place-items-center rounded-full bg-slate-900 text-white transition-colors disabled:bg-slate-200 disabled:text-slate-400 dark:bg-white dark:text-slate-900 dark:disabled:bg-raised dark:disabled:text-muted";
const plusButton = "pointer-events-auto [-webkit-tap-highlight-color:transparent] grid size-9 shrink-0 place-items-center rounded-full bg-slate-100 text-slate-600 dark:bg-raised dark:text-text-secondary";

// MobileComposer is the phone message box shared by the start screen and the
// threads: one line pinned to the bottom with +, the field and the actions.
// The + opens the extras floating above it, so nothing else moves. While
// writing or dictating, the field smoothly expands out from between the
// buttons onto its own full-width line above them; the same node stays
// mounted, so focus and keyboard never drop.
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
  const plusRef = useRef<HTMLButtonElement>(null);
  const actionsRef = useRef<HTMLDivElement>(null);
  const barRef = useRef<HTMLDivElement>(null);
  const [insets, setInsets] = useState({ left: 38, right: 112, bar: 36 });
  // Like a pop-up: a tap anywhere else closes the extras and folds the field.
  // Folding on blur instead would move the buttons under a finger mid-tap.
  const box = useOutside(open || focused, () => { setOpen(false); setFocused(false); });
  const expanded = focused || !!active;

  useLayoutEffect(() => {
    const plus = plusRef.current;
    const actionsEl = actionsRef.current;
    const bar = barRef.current;
    if (!plus || !actionsEl || !bar) return;
    const measure = () => {
      const left = Math.ceil(plus.getBoundingClientRect().width) + 2;
      const right = Math.ceil(actionsEl.getBoundingClientRect().width) + 2;
      const barH = Math.ceil(bar.getBoundingClientRect().height) || 36;
      setInsets((prev) => (prev.left === left && prev.right === right && prev.bar === barH ? prev : { left, right, bar: barH }));
    };
    measure();
    const ro = new ResizeObserver(measure);
    ro.observe(plus);
    ro.observe(actionsEl);
    ro.observe(bar);
    return () => ro.disconnect();
  }, []);

  return (
    <div ref={box} className="relative">
      {open && (
        <div className="absolute inset-x-0 bottom-full z-20 mb-2 flex origin-bottom animate-[extras-in_160ms_ease-out] flex-wrap items-center gap-1 rounded-2xl border border-slate-200 bg-white p-2 text-xs text-slate-500 shadow-sm dark:border-border dark:bg-surface dark:text-muted">
          {extras(() => setOpen(false))}
        </div>
      )}
      <form {...form} className={composerFrame}>
        {top}
        <div className="flex flex-col p-1.5">
          {/* Collapsed, a negative bottom margin pulls the button row behind the
              field while left/right margins nestle it between + and the actions.
              Expanding transitions those margins to a full-width row above the
              buttons so the screen above rises gradually instead of jumping. */}
          <div
            onFocus={() => { setOpen(false); setFocused(true); }}
            style={{
              marginBottom: expanded ? 4 : -insets.bar,
              marginLeft: expanded ? 0 : insets.left,
              marginRight: expanded ? 0 : insets.right,
            }}
            className={`flex min-h-9 min-w-0 flex-col justify-center transition-[margin,padding] duration-200 ease-[cubic-bezier(0.22,1,0.36,1)] motion-reduce:transition-none ${expanded ? "px-2" : "px-0"}`}
          >
            <div className="relative min-w-0 w-full">{field}</div>
          </div>
          <div ref={barRef} className="pointer-events-none flex items-end justify-between gap-0.5">
            <button ref={plusRef} type="button" aria-label="Más opciones" aria-expanded={open} onClick={() => setOpen(!open)} className={plusButton}>
              <Plus weight="regular" className={`size-5 transition-transform duration-200 ${open ? "rotate-45" : ""}`} />
            </button>
            <div ref={actionsRef} className="pointer-events-auto ml-auto flex items-end gap-0.5">{actions}</div>
          </div>
        </div>
        {below}
      </form>
    </div>
  );
}
