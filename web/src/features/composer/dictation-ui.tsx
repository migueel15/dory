import { useEffect, useLayoutEffect, useRef, useState, type ReactNode, type RefObject } from "react";
import { CircleNotch, Microphone } from "@phosphor-icons/react";
import { formatChord, isTextInput, useDictationShortcutSetting } from "./dictation-shortcut";
import { iconButton } from "../../components/ui";
import { events } from "../../lib/store";
import { Phase, useDictationInProgress, meterBars } from "./local-dictation";

export function DictationPreview({ value, range, format, className = "" }: {
  value: string;
  range: { start: number; end: number };
  format?: (text: string) => ReactNode;
  className?: string;
}) {
  const viewport = useRef<HTMLDivElement>(null);
  const provisional = useRef<HTMLSpanElement>(null);
  useLayoutEffect(() => {
    const box = viewport.current;
    const text = provisional.current;
    if (!box || !text) return;
    const bottom = text.getBoundingClientRect().bottom;
    const visibleBottom = box.getBoundingClientRect().bottom;
    if (bottom > visibleBottom) box.scrollTop += bottom - visibleBottom;
  }, [value]);
  const before = value.slice(0, range.start);
  const after = value.slice(range.end);
  return (
    <div ref={viewport} aria-hidden="true" className={`max-h-60 overflow-y-auto whitespace-pre-wrap break-words px-4 pb-1 pt-3.5 text-[0.9375rem] leading-6 ${className}`}>
      {format ? format(before) : before}
      <span ref={provisional} className="italic text-slate-500 dark:text-muted">{value.slice(range.start, range.end)}</span>
      {format ? format(after) : after}
    </div>
  );
}

export type DictationControl = {
  phase: Phase;
  active: boolean;
  model?: { name: string };
  meter: RefObject<HTMLSpanElement | null>;
  toggle: () => void;
};

export function DictationTimer() {
  const [seconds, setSeconds] = useState(0);

  useEffect(() => {
    const startedAt = performance.now();
    const timer = window.setInterval(() => {
      setSeconds(Math.floor((performance.now() - startedAt) / 1000));
    }, 1000);
    return () => window.clearInterval(timer);
  }, []);

  const minutes = Math.floor(seconds / 60);
  return <span className="text-xs tabular-nums">{minutes}:{String(seconds % 60).padStart(2, "0")}</span>;
}

export function DictationMic({ dictation, disabled, round = false }: {
  dictation: DictationControl;
  disabled?: boolean;
  // round: the phone box's 36 px circular buttons.
  round?: boolean;
}) {
  const { phase, model } = dictation;
  const shortcut = formatChord(useDictationShortcutSetting());
  const activeElsewhere = useDictationInProgress() && !dictation.active;
  const recording = phase === "recording";
  const busy = phase === "starting" || phase === "finishing";
  const label = recording ? "Parar dictado" : phase === "starting" ? "Preparando dictado" : phase === "finishing" ? "Transcribiendo" : "Dictar";
  const unavailable = disabled || busy || activeElsewhere || !model;
  const hint = activeElsewhere ? "Termina el otro dictado para usar este micrófono" : !model ? "Configura un modelo de dictado en Ajustes" : phase === "idle" ? `Dictar con ${model.name}` : label;
  // The button grows while recording (level meter and timer): the wrapper
  // follows its width with a transition, so the buttons beside it slide
  // instead of jumping.
  const button = useRef<HTMLButtonElement>(null);
  const [width, setWidth] = useState<number>();
  useLayoutEffect(() => {
    const el = button.current;
    if (!el) return;
    const measure = () => setWidth(el.offsetWidth);
    measure();
    const resize = new ResizeObserver(measure);
    resize.observe(el);
    return () => resize.disconnect();
  }, []);
  return (
    <span title={`${hint} (${shortcut})`} style={{ width }} className="inline-flex shrink-0 overflow-hidden transition-[width] duration-200 ease-out motion-reduce:transition-none">
    <button ref={button} type="button" aria-label={label} aria-pressed={recording} disabled={unavailable} onClick={dictation.toggle}
      className={`${recording ? `${iconButton} gap-1.5 px-2` : iconButton} ${round ? "!h-9 !min-w-9 !rounded-full bg-slate-100 dark:bg-raised" : ""} shrink-0 disabled:pointer-events-none`}>
      {recording ? (
        <>
          <span ref={dictation.meter} aria-hidden="true" className="flex h-4 items-center gap-0.5">
            {Array.from({ length: meterBars }, (_, index) => (
              <span key={index} className="h-4 w-0.5 rounded-full bg-current [transform:scaleY(var(--meter-level,0.16))] transition-transform duration-100 ease-out motion-reduce:transition-none" />
            ))}
          </span>
          <DictationTimer />
        </>
      ) : busy ? (
        <CircleNotch weight="regular" aria-hidden="true" className="size-4 animate-spin" />
      ) : (
        <Microphone weight="regular" aria-hidden="true" className="size-4" />
      )}
    </button>
    </span>
  );
}

// useEnterFinishesDictation makes Enter (and Ctrl/Cmd+Enter) work while
// dictating in a field that does not handle it itself: the key finishes the
// dictation, and once the text is in it is pressed again on the field, so the
// field's own shortcut (send, save, publish…) runs with the final text.
// Shift+Enter alone still only adds a line. The key counts when the focus is
// in the field or elsewhere in its dictation panel (the microphone button).
export function useEnterFinishesDictation(dictation: { active: boolean; finishAndSubmit: (submit: (text: string) => void) => void }, area: RefObject<HTMLTextAreaElement | null>) {
  const pending = useRef<KeyboardEventInit | null>(null);
  const latest = useRef(dictation);
  latest.current = dictation;
  useEffect(() => {
    if (!dictation.active) return;
    const handle = (e: KeyboardEvent) => {
      if (e.key !== "Enter" || e.isComposing || (e.shiftKey && !e.ctrlKey && !e.metaKey)) return;
      const box = area.current;
      const target = e.target instanceof Element ? e.target : null;
      const panel = box?.closest("[data-dictation-panel]");
      if (!box || !target || (target !== box && !(panel?.contains(target) && !isTextInput(target)))) return;
      e.preventDefault();
      e.stopPropagation();
      if (e.repeat || pending.current) return;
      pending.current = { key: "Enter", code: "Enter", ctrlKey: e.ctrlKey, metaKey: e.metaKey, shiftKey: e.shiftKey, altKey: e.altKey };
      latest.current.finishAndSubmit(() => {});
    };
    window.addEventListener("keydown", handle, true);
    return () => window.removeEventListener("keydown", handle, true);
  }, [dictation.active, area]);
  useEffect(() => {
    if (!pending.current || dictation.active) return;
    const init = pending.current;
    pending.current = null;
    const box = area.current;
    if (!box) return;
    box.focus();
    box.dispatchEvent(new KeyboardEvent("keydown", { ...init, bubbles: true, cancelable: true }));
  });
}

