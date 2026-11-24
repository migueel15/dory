import { useEffect, useRef, useState } from "react";
import { agentNote } from "../chat/subagents";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { Lightning, PencilSimple, Trash } from "@phosphor-icons/react";
import { api, type Attachment, type ThreadView } from "../../api/client";
import { Button, ErrorText } from "../../components/ui";
import { DictationField } from "./dictation-field";
import { useDictationInProgress } from "./local-dictation";
import { iconBtn } from "../chat/chat";
import { MessageParts } from "../chat/message-parts";
import { HoverCard } from "../../components/tooltip";
import { CodeRefCard } from "../chat/code-ref";
import type { DraftQuote } from "../chat/quote-selection";
import { qk } from "../../api/queries/keys";

// Queue shows the messages waiting for the running turn as outlined bubbles,
// each with its actions underneath.
export function Queue({ t }: { t: ThreadView }) {
  const qc = useQueryClient();
  const act = useMutation({
    mutationFn: ({ run, action, text }: { run: string; action: "edit" | "cancel" | "steer"; text?: string }) => api.queue(t.id, run, action, text),
    onSuccess: () => qc.invalidateQueries({ queryKey: qk.thread(t.id) }),
  });
  if (!t.queue.length) return null;
  return (
    <section aria-label="Mensajes en cola" className="space-y-3 border-t border-dashed border-slate-300 pt-3 dark:border-border">
      {t.queue.map((r) => <QueuedRow key={r.run_id} text={r.text} files={r.files ?? []} canSteer={t.running}
        onSteer={() => act.mutate({ run: r.run_id, action: "steer" })}
        onCancel={() => act.mutate({ run: r.run_id, action: "cancel" })}
        onEdit={(text) => act.mutate({ run: r.run_id, action: "edit", text })} />)}
      <ErrorText error={act.error} />
    </section>
  );
}

export function QueuedRow({ text, files, canSteer, onSteer, onCancel, onEdit }: { text: string; files: Attachment[]; canSteer: boolean; onSteer: () => void; onCancel: () => void; onEdit: (text: string) => void }) {
  // A result from another agent waits as its one-line summary.
  const note = agentNote({ id: "", role: "user", text, streaming: false, created_at: "" });
  const [draft, setDraft] = useState<string | null>(null);
  const dictating = useDictationInProgress();
  const save = () => {
    if (dictating || draft === null) return;
    onEdit(draft);
    setDraft(null);
  };
  return (
    <div className="msg-in flex flex-col items-end gap-1">
      <div className="w-fit max-w-[85%] rounded-2xl rounded-br-md border border-dashed border-slate-400 px-4 py-2.5 text-slate-700 dark:border-border-strong dark:text-text-secondary">
        {draft === null ? <div className="flex flex-col text-sm">{note ? <span className="truncate">{note.state ? `Subagente «${note.title}» ${note.state}` : `Mensaje de «${note.title}»`}</span> : <MessageParts text={text} />}</div> : (
          <form className="space-y-2" onSubmit={(e) => { e.preventDefault(); save(); }}>
            <DictationField autoFocus rows={2} aria-label="Editar mensaje en cola" value={draft} onValue={setDraft} onKeyDown={(e) => {
              if (e.key === "Escape") setDraft(null);
              if (e.key === "Enter" && !e.shiftKey && !e.nativeEvent.isComposing) { e.preventDefault(); save(); }
            }} />
            <Button disabled={dictating}>Guardar</Button>
          </form>
        )}
        {files.length > 0 && <p className="mt-1 text-xs opacity-70">{files.map((f) => f.name).join(", ")}</p>}
      </div>
      <div className="flex items-center gap-0.5">
        <button type="button" className={iconBtn} title="Mandar ya: para el turno en curso y lo envía" disabled={!canSteer} onClick={onSteer}><Lightning /></button>
        <button type="button" className={iconBtn} title="Editar" onClick={() => setDraft(text)}><PencilSimple /></button>
        <button type="button" className={iconBtn} title="Eliminar de la cola" onClick={onCancel}><Trash /></button>
      </div>
    </div>
  );
}

// useCodeTip shows a code reference of the message box in full when the
// pointer rests on it, after the same wait as a tooltip: the file and lines,
// the code as it is in the file, and the comment, each in its own part.
export function useCodeTip() {
  const [tip, setTip] = useState<{ el: HTMLElement; quote: DraftQuote } | null>(null);
  const wait = useRef<ReturnType<typeof setTimeout>>(undefined);
  useEffect(() => () => clearTimeout(wait.current), []);
  const show = (el: HTMLElement, quote: DraftQuote) => { clearTimeout(wait.current); if (tip?.el === el) return; wait.current = setTimeout(() => setTip({ el, quote }), 700); };
  // Leaving the reference waits a moment, so the pointer can reach the card and scroll its code.
  const hide = () => { clearTimeout(wait.current); wait.current = setTimeout(() => setTip(null), 200); };
  const keep = () => clearTimeout(wait.current);
  const code = tip?.quote.code;
  const card = tip && code && <HoverCard anchor={tip.el} interactive onPointerEnter={keep} onPointerLeave={hide}><CodeRefCard data={{ ...code, code: tip.quote.text, comment: tip.quote.reply }} /></HoverCard>;
  return { show, hide, card };
}

