import { useEffect, useLayoutEffect, useRef, useState } from "react";
import { createPortal } from "react-dom";
import { ArrowBendUpLeft, Microphone, Plus, Quotes, X } from "@phosphor-icons/react";
import { useLocalDictation } from "../composer/local-dictation";
import { useSpeechModel, useSpeechModels } from "../composer/speech-models";
import { DictationMic, DictationPreview } from "../composer/dictation-ui";
import { hasDictationPopupOutside, isTextInput, useDictationShortcut } from "../composer/dictation-shortcut";
import { ErrorText } from "../../components/ui";

// A quote is a piece of the chat or, with code, lines of a file in the diff.
export type CodeRef = { path: string; start: number; end: number };
export type DraftQuote = { id: string; text: string; reply: string; code?: CodeRef };
type Selection = { text: string; range: Range };

const quoteAction = "inline-flex h-8 shrink-0 items-center gap-1.5 rounded-md px-2 text-xs font-medium text-slate-700 hover:bg-slate-100 dark:text-text-secondary dark:hover:bg-raised";

function readSelection(): Selection | undefined {
  const selected = document.getSelection();
  const text = selected?.toString().trim();
  const range = selected?.rangeCount ? selected.getRangeAt(0) : undefined;
  const source = range?.commonAncestorContainer;
  const element = source instanceof Element ? source : source?.parentElement;
  return text && range && element && !element.closest("[data-quote-ui]") ? { text, range: range.cloneRange() } : undefined;
}

export function QuoteSelection({ onQuote }: { onQuote: (text: string, reply: string) => void }) {
  const [selection, setSelection] = useState<Selection>();
  const [editing, setEditing] = useState<"manual" | "dictate">();
  const catalog = useSpeechModels();
  const model = useSpeechModel(catalog.data);
  const panel = useRef<HTMLDivElement>(null);

  const dismiss = () => {
    document.getSelection()?.removeAllRanges();
    setSelection(undefined);
    setEditing(undefined);
  };
  const append = (note: string) => {
    if (!selection) return;
    const { text } = selection;
    dismiss();
    onQuote(text, note.trim());
  };

  useEffect(() => {
    if (editing) return;
    // While the mouse button is held down the selection is still being dragged:
    // the actions appear only once it is released.
    let pressed = false;
    const update = () => { if (!pressed) setSelection(readSelection()); };
    const down = (event: PointerEvent) => {
      if (event.target instanceof Element && event.target.closest("[data-quote-ui]")) return;
      pressed = true;
      setSelection(undefined);
    };
    const up = () => {
      if (!pressed) return;
      pressed = false;
      setSelection(readSelection());
    };
    document.addEventListener("selectionchange", update);
    document.addEventListener("pointerdown", down, true);
    document.addEventListener("pointerup", up, true);
    document.addEventListener("pointercancel", up, true);
    return () => {
      document.removeEventListener("selectionchange", update);
      document.removeEventListener("pointerdown", down, true);
      document.removeEventListener("pointerup", up, true);
      document.removeEventListener("pointercancel", up, true);
    };
  }, [editing]);

  useDictationShortcut((event, context) => {
    if (context !== "selection" || editing || !model || !(event.target instanceof Element) ||
      isTextInput(event.target) || hasDictationPopupOutside()) return false;
    const quote = readSelection();
    if (!quote) return false;
    setSelection(quote);
    setEditing("dictate");
    return true;
  });

  useLayoutEffect(() => {
    const popup = panel.current;
    if (!selection || !popup) return;
    const place = () => {
      const rect = selection.range.getBoundingClientRect();
      const margin = 8;
      const { offsetWidth: width, offsetHeight: height } = popup;
      const viewportWidth = document.documentElement.clientWidth;
      const viewportHeight = document.documentElement.clientHeight;
      const x = rect.left + (rect.width - width) / 2;
      const y = rect.top - height - margin >= margin ? rect.top - height - margin : rect.bottom + margin;
      popup.style.left = `${Math.max(margin, Math.min(x, viewportWidth - width - margin))}px`;
      popup.style.top = `${Math.max(margin, Math.min(y, viewportHeight - height - margin))}px`;
      popup.style.visibility = editing || (rect.bottom > 0 && rect.top < viewportHeight) ? "visible" : "hidden";
    };
    place();
    const observer = new ResizeObserver(place);
    observer.observe(popup);
    window.addEventListener("resize", place);
    window.addEventListener("scroll", place, true);
    return () => {
      observer.disconnect();
      window.removeEventListener("resize", place);
      window.removeEventListener("scroll", place, true);
    };
  }, [selection, editing]);

  useEffect(() => {
    if (!selection) return;
    const escape = (event: KeyboardEvent) => {
      if (event.key === "Escape" && !event.isComposing && !event.defaultPrevented) {
        event.preventDefault();
        dismiss();
      }
    };
    document.addEventListener("keydown", escape);
    return () => document.removeEventListener("keydown", escape);
  }, [selection]);

  if (!selection) return null;
  return createPortal(
    <div ref={panel} data-quote-ui role={editing ? "dialog" : "group"} aria-label={editing ? "Responder a la cita" : "Acciones de la selección"}
      className={`fixed z-50 max-w-[calc(100vw-1rem)] rounded-xl border border-slate-200 bg-white shadow-lg dark:border-border-strong dark:bg-surface ${editing ? "w-[min(26rem,calc(100vw-1rem))] p-3" : "flex gap-0.5 p-1"}`}>
      {editing ? (
        <QuoteReply autoDictate={editing === "dictate"} text={selection.text} onCancel={dismiss} onAdd={append} />
      ) : (
        <>
          <button type="button" onMouseDown={(event) => event.preventDefault()} onClick={() => append("")} className={quoteAction}>
            <Quotes weight="thin" className="size-4" />Citar
          </button>
          <button type="button" onMouseDown={(event) => event.preventDefault()} onClick={() => setEditing("manual")} className={quoteAction}>
            <ArrowBendUpLeft weight="thin" className="size-4" />Responder
          </button>
          {model && <button type="button" title="Responder dictando" aria-label="Responder dictando" onMouseDown={(event) => event.preventDefault()} onClick={() => setEditing("dictate")} className={`${quoteAction} !px-2`}>
            <Microphone weight="thin" className="size-4" />
          </button>}
        </>
      )}
    </div>,
    document.body,
  );
}


function QuoteReply({ autoDictate, text, onCancel, onAdd }: { autoDictate: boolean; text: string; onCancel: () => void; onAdd: (reply: string) => void }) {
  const [reply, setReply] = useState("");
  const input = useRef<HTMLTextAreaElement>(null);
  const dictation = useLocalDictation(reply, setReply, input);
  const showingDictation = !!dictation.preview;

  useEffect(() => {
    if (!autoDictate) return;
    const scheduled = window.setTimeout(() => void dictation.start(), 0);
    return () => window.clearTimeout(scheduled);
  }, [autoDictate]); // eslint-disable-line react-hooks/exhaustive-deps

  useLayoutEffect(() => {
    const area = input.current;
    if (!area || showingDictation) return;
    const resize = () => {
      area.style.height = "auto";
      area.style.height = `${area.scrollHeight}px`;
    };
    resize();
    window.addEventListener("resize", resize);
    return () => window.removeEventListener("resize", resize);
  }, [reply, showingDictation]);

  const add = () => {
    if (dictation.active) dictation.finishAndSubmit(onAdd);
    else onAdd(reply);
  };
  return (
    <form data-dictation-panel onSubmit={(event) => { event.preventDefault(); add(); }}>
      <div className="mb-2 flex items-start gap-2">
        <blockquote className="line-clamp-2 min-w-0 flex-1 border-l-2 border-slate-300 pl-2 text-sm text-slate-600 dark:border-border-strong dark:text-muted">{text}</blockquote>
        <button type="button" onClick={onCancel} title="Cancelar respuesta" aria-label="Cancelar respuesta" className="grid size-8 shrink-0 place-items-center text-slate-500 hover:text-slate-900 dark:text-muted dark:hover:text-white">
          <X weight="thin" className="size-4" />
        </button>
      </div>
      <div className="flex items-end gap-2">
        {dictation.preview && <DictationPreview value={dictation.value} range={dictation.preview} className="min-w-0 flex-1 !max-h-36 !px-1 !py-1.5 !text-sm !leading-5" />}
        <textarea ref={input} autoFocus rows={1} value={dictation.value} readOnly={dictation.active} aria-label="Respuesta a la cita" placeholder="Responder a la cita…"
          onChange={(event) => setReply(event.target.value)}
          onKeyDown={(event) => {
            if (event.key === "Enter" && !event.shiftKey && !event.repeat && !event.nativeEvent.isComposing) {
              event.preventDefault();
              add();
            }
          }}
          className={showingDictation ? "sr-only" : "max-h-36 min-w-0 flex-1 resize-none overflow-y-auto bg-transparent px-1 py-1.5 text-sm leading-5 outline-none! focus-visible:outline-none!"} />
        <DictationMic dictation={dictation} />
        <button type="submit" disabled={dictation.phase === "finishing"} className={`${quoteAction} disabled:opacity-40`} title={dictation.active ? "Terminar el dictado y añadir" : "Añadir al mensaje"}>
          <Plus weight="thin" className="size-4" />Añadir
        </button>
      </div>
      <ErrorText error={dictation.error} />
    </form>
  );
}
