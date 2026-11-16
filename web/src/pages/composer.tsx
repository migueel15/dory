import { useEffect, useLayoutEffect, useRef, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { ArrowUp, CaretDown, Check, File as FileIcon, FileText, Lightning, ListBullets, Paperclip, PencilSimple, Stop, Trash, X } from "@phosphor-icons/react";
import { api, type Attachment, type ThreadView } from "../api/client";
import { Button, ErrorText, Input, errMsg, uuid } from "../components/ui";
import { ProviderIcon } from "../components/launch-picker";
import { saveEffort, savedEffort } from "../effort";
import { iconBtn } from "./chat";

// The composer of a thread, kept from Dory v1: attachments, @ mentions of
// files, model, effort and permissions, and send, queue or stop.

export const runtimeModes = [
  { value: "approval-required", label: "Pedir permiso" },
  { value: "full-access", label: "Acceso total" },
];

export type MenuOption = { value: string; label: string; group?: string; icon?: React.ReactNode };

// Insert adds text to the composer from outside it, such as a quoted selection.
export type Insert = { text: string; seq: number };

// Chip is a compact dropdown for the composer toolbar; the menu opens upwards.
export function Chip({ value, options, onChange, title, icon }: { value: string; options: MenuOption[]; onChange: (v: string) => void; title: string; icon?: React.ReactNode }) {
  const [open, setOpen] = useState(false);
  const box = useOutside(open, () => setOpen(false));
  const current = options.find((o) => o.value === value);
  return (
    <div ref={box} className="relative">
      <button type="button" title={title} aria-haspopup="listbox" aria-expanded={open} onClick={() => setOpen(!open)}
        onKeyDown={(e) => { if (e.key === "Escape") setOpen(false); }}
        className="inline-flex h-8 max-w-[11rem] items-center gap-1.5 rounded-full px-2.5 text-xs font-medium text-slate-600 transition-colors hover:bg-slate-100 hover:text-slate-900 dark:text-text-secondary dark:hover:bg-raised dark:hover:text-white">
        {current?.icon ?? icon}<span className="truncate">{current?.label ?? value}</span><CaretDown weight="regular" className="size-3 shrink-0 opacity-60" />
      </button>
      {open && (
        <ul role="listbox" className="absolute bottom-full left-0 z-20 mb-2 max-h-72 min-w-[12rem] overflow-y-auto rounded-xl border border-slate-200 bg-white p-1 shadow-lg dark:border-border dark:bg-surface">
          {options.map((o) => (
            <li key={o.value}>
              <button type="button" role="option" aria-selected={o.value === value} onClick={() => { onChange(o.value); setOpen(false); }}
                className="flex w-full items-center gap-2 rounded-lg px-2 py-1.5 text-left text-sm hover:bg-slate-100 dark:hover:bg-raised">
                {o.icon}<span className="min-w-0 flex-1 truncate">{o.label}</span>{o.value === value && <Check weight="regular" className="size-3.5" />}
              </button>
            </li>
          ))}
        </ul>
      )}
    </div>
  );
}

function useOutside(open: boolean, close: () => void) {
  const box = useRef<HTMLDivElement>(null);
  useEffect(() => {
    if (!open) return;
    const onDown = (e: PointerEvent) => { if (!box.current?.contains(e.target as Node)) close(); };
    document.addEventListener("pointerdown", onDown);
    return () => document.removeEventListener("pointerdown", onDown);
  }, [open]); // eslint-disable-line react-hooks/exhaustive-deps
  return box;
}

// ModelPicker joins model and effort in one chip: models on the left and, on
// the right, the efforts of the model last clicked.
export function ModelPicker({ models, selected, effort, effortsOf, onChange }: { models: MenuOption[]; selected: string; effort: string; effortsOf: (value: string) => { values: string[]; initial: string } | undefined; onChange: (value: string, effort: string) => void }) {
  const [open, setOpen] = useState(false);
  const [focus, setFocus] = useState(selected);
  const box = useOutside(open, () => setOpen(false));
  useEffect(() => { if (open) setFocus(selected); }, [open, selected]);
  const current = models.find((m) => m.value === selected);
  const efforts = effortsOf(focus);
  const pickModel = (value: string) => {
    const e = effortsOf(value);
    setFocus(value);
    onChange(value, e?.initial ?? "");
    if (!e) setOpen(false);
  };
  const row = "flex w-full items-center gap-2 rounded-lg px-2 py-1.5 text-left text-sm hover:bg-slate-100 dark:hover:bg-raised";
  return (
    <div ref={box} className="relative">
      <button type="button" title="Modelo y esfuerzo" aria-haspopup="dialog" aria-expanded={open} onClick={() => setOpen(!open)}
        onKeyDown={(e) => { if (e.key === "Escape") setOpen(false); }}
        className="inline-flex h-8 max-w-[15rem] items-center gap-1.5 rounded-full px-2.5 text-xs font-medium text-slate-600 transition-colors hover:bg-slate-100 hover:text-slate-900 dark:text-text-secondary dark:hover:bg-raised dark:hover:text-white">
        {current?.icon}<span className="truncate">{current?.label ?? selected}</span>
        {effort && <span className="shrink-0 text-slate-400">· {effort}</span>}<CaretDown weight="regular" className="size-3 shrink-0 opacity-60" />
      </button>
      {open && (
        <div role="dialog" aria-label="Modelo y esfuerzo" className="absolute bottom-full left-0 z-20 mb-2 flex max-h-80 w-[min(30rem,calc(100vw-2rem))] overflow-hidden rounded-xl border border-slate-200 bg-white shadow-lg dark:border-border dark:bg-surface">
          <ul className="min-w-0 flex-1 overflow-y-auto p-1">
            {models.map((o) => (
              <li key={o.value}>
                <button type="button" onClick={() => pickModel(o.value)} className={`${row} ${o.value === focus ? "bg-slate-100 dark:bg-raised" : ""}`}>
                  {o.icon}<span className="min-w-0 flex-1 truncate">{o.label}</span>{o.value === selected && <Check weight="regular" className="size-3.5" />}
                </button>
              </li>
            ))}
          </ul>
          <div className="w-32 shrink-0 overflow-y-auto border-l border-slate-200 p-1 dark:border-border">
            <p className="px-2 pb-1 pt-2 text-[11px] font-semibold uppercase tracking-wide text-slate-400">Esfuerzo</p>
            {efforts ? efforts.values.map((x) => {
              const active = focus === selected ? x === effort : x === efforts.initial;
              return (
                <button key={x} type="button" onClick={() => { onChange(focus, x); setOpen(false); }} className={`${row} ${active ? "font-medium" : ""}`}>
                  <span className="min-w-0 flex-1 truncate">{x}</span>{active && <Check weight="regular" className="size-3.5" />}
                </button>
              );
            }) : <p className="px-2 py-1.5 text-xs text-slate-400">Sin opciones</p>}
          </div>
        </div>
      )}
    </div>
  );
}

type Draft = { key: string; name: string; mime: string; preview: string; done?: Attachment; error?: string };

export function Composer({ t, onSent, insert }: { t: ThreadView; onSent: () => void; insert?: Insert }) {
  const qc = useQueryClient();
  const key = ["thread", t.id];
  const providers = useQuery({ queryKey: ["node-providers", t.node_id], queryFn: () => api.nodeProviders(t.node_id), staleTime: 60_000 });
  const [text, setText] = useState("");
  const area = useRef<HTMLTextAreaElement>(null);
  const picker = useRef<HTMLInputElement>(null);
  const [files, setFiles] = useState<Draft[]>([]);
  const uploading = files.some((f) => !f.done && !f.error);
  useLayoutEffect(() => { const el = area.current; if (el) { el.style.height = "auto"; el.style.height = `${Math.min(el.scrollHeight, 240)}px`; } }, [text]);
  // Text sent from outside, such as a quoted selection, goes before the cursor.
  useEffect(() => {
    if (!insert) return;
    setText((x) => (x && !x.endsWith("\n") ? `${x}\n` : x) + insert.text);
    area.current?.focus();
  }, [insert?.seq]); // eslint-disable-line react-hooks/exhaustive-deps
  // Each file uploads as soon as it is added; the message carries the results.
  const addFiles = (list: FileList | File[]) => {
    for (const file of Array.from(list)) {
      const k = uuid();
      const named = file.name ? file : new File([file], `imagen-${new Date().toISOString().slice(11, 19).replace(/:/g, "")}.png`, { type: file.type });
      const preview = URL.createObjectURL(named);
      setFiles((fs) => [...fs, { key: k, name: named.name, mime: named.type, preview }]);
      api.upload(t.id, named).then(
        (done) => setFiles((fs) => fs.map((f) => (f.key === k ? { ...f, done } : f))),
        (e: unknown) => setFiles((fs) => fs.map((f) => (f.key === k ? { ...f, error: errMsg(e) || "No se pudo subir" } : f))),
      );
    }
  };
  const removeFile = (k: string) => setFiles((fs) => fs.filter((f) => { if (f.key === k) URL.revokeObjectURL(f.preview); return f.key !== k; }));
  const send = useMutation({
    mutationFn: () => api.send(t.id, text.trim(), files.flatMap((f) => (f.done ? [f.done.id] : []))),
    onSuccess: () => { setText(""); files.forEach((f) => URL.revokeObjectURL(f.preview)); setFiles([]); onSent(); qc.invalidateQueries({ queryKey: key }); },
  });
  const stop = useMutation({ mutationFn: () => api.interrupt(t.id) });
  const settings = useMutation({
    mutationFn: (body: Parameters<typeof api.threadSettings>[1]) => api.threadSettings(t.id, body),
    onSuccess: () => qc.invalidateQueries({ queryKey: key }),
  });
  const suggest = useFileSuggest(t.id, text, area, setText);
  const canSend = (!!text.trim() || files.some((f) => f.done)) && !uploading && !send.isPending;
  const submit = () => { if (canSend) send.mutate(); };
  if (!t.local) return null;
  const busy = t.running;
  const provider = providers.data?.find((p) => p.id === t.provider);
  const models: MenuOption[] = (provider?.models ?? [{ slug: t.model, name: t.model || t.provider }]).map((m) => ({ value: m.slug, label: m.name, icon: <ProviderIcon driver={t.provider} model={m.slug} /> }));
  return (
    <div className="mx-auto w-full max-w-3xl px-3 pb-4 pt-2 sm:px-6">
      <form onSubmit={(e) => { e.preventDefault(); submit(); }}
        onDragOver={(e) => { if (e.dataTransfer.types.includes("Files")) e.preventDefault(); }}
        onDrop={(e) => { if (e.dataTransfer.files.length) { e.preventDefault(); addFiles(e.dataTransfer.files); } }}
        onPaste={(e) => {
          // Screenshots often come only as clipboard items, not in .files.
          const fs = Array.from(e.clipboardData.items).flatMap((i) => { const f = i.kind === "file" ? i.getAsFile() : null; return f ? [f] : []; });
          if (fs.length) { e.preventDefault(); addFiles(fs); }
        }}
        className="relative z-10 rounded-3xl border border-slate-200 bg-white shadow-md dark:border-border-strong dark:bg-surface">
        {suggest.menu}
        {files.length > 0 && (
          <div className="flex flex-wrap gap-2 px-3 pt-3">
            {files.map((f) => (
              <div key={f.key} title={f.error ?? f.name} className={`group relative size-14 shrink-0 overflow-hidden rounded-xl border bg-slate-50 dark:bg-raised ${f.error ? "border-red-400" : "border-slate-200 dark:border-border"}`}>
                <a href={f.preview} target="_blank" rel="noreferrer" aria-label={`Ver ${f.name}`} className="block size-14 cursor-zoom-in">
                  {f.mime.startsWith("image/") ? <img src={f.preview} alt="" className="size-14 shrink-0 object-cover" /> : <span className="grid size-14 shrink-0 place-items-center text-slate-400"><FileText className="size-6" /></span>}
                </a>
                {f.error && <span className="absolute inset-x-0 bottom-0 bg-red-500 text-center text-[9px] font-medium text-white">Error</span>}
                {!f.done && !f.error && <span className="absolute inset-0 grid place-items-center bg-white/60 dark:bg-black/40"><span className="size-4 animate-spin rounded-full border-2 border-current border-t-transparent" /></span>}
                <button type="button" onClick={() => removeFile(f.key)} aria-label={`Quitar ${f.name}`} className="absolute right-1 top-1 grid size-5 place-items-center rounded-full bg-slate-900/70 text-white transition-opacity hover:bg-slate-900 sm:opacity-0 sm:group-hover:opacity-100 sm:focus-visible:opacity-100"><X weight="regular" className="size-3" /></button>
              </div>
            ))}
          </div>
        )}
        <textarea ref={area} rows={1} value={text} onChange={(e) => setText(e.target.value)} onSelect={suggest.track} onClick={suggest.track} aria-label="Mensaje"
          placeholder={t.status === "waiting" ? "Escribe tu respuesta…" : "Escribe al hilo…"}
          onKeyDown={(e) => { if (e.nativeEvent.isComposing || suggest.onKeyDown(e)) return; if (e.key === "Enter" && !e.shiftKey && window.matchMedia("(min-width: 640px)").matches) { e.preventDefault(); submit(); } }}
          className="block max-h-60 w-full resize-none break-words bg-transparent px-4 pb-1 pt-3.5 text-[15px] leading-6 !outline-none placeholder:text-slate-400" />
        <div className="flex items-center gap-0.5 px-2 pb-2">
          <div className="flex min-w-0 flex-1 flex-wrap items-center gap-0.5">
            <input ref={picker} type="file" multiple hidden onChange={(e) => { if (e.target.files) addFiles(e.target.files); e.target.value = ""; }} />
            <button type="button" onClick={() => picker.current?.click()} title="Adjuntar imágenes o ficheros" aria-label="Adjuntar"
              className="grid size-8 place-items-center rounded-full text-slate-500 hover:bg-slate-100 hover:text-slate-900 dark:text-text-secondary dark:hover:bg-raised dark:hover:text-white">
              <Paperclip weight="regular" className="size-4" />
            </button>
            <ModelPicker models={models} selected={t.model} effort={t.effort}
              effortsOf={(value) => { const m = provider?.models.find((x) => x.slug === value); return m?.efforts?.length ? { values: m.efforts, initial: value === t.model ? t.effort : savedEffort(value, m.efforts) || m.efforts[0] } : undefined; }}
              onChange={(model, effort) => { if (effort) saveEffort(model, effort); settings.mutate({ model, effort }); }} />
            <Chip title="Permisos" value={t.runtime_mode} options={runtimeModes} onChange={(x) => settings.mutate({ runtime_mode: x as ThreadView["runtime_mode"] })} />
          </div>
          {busy && !text.trim() && !files.length ? (
            <button type="button" onClick={() => stop.mutate()} disabled={stop.isPending} title="Parar el turno en curso" aria-label="Parar"
              className="grid size-9 shrink-0 place-items-center rounded-full bg-slate-900 text-white transition hover:bg-slate-700 disabled:opacity-50 dark:bg-white dark:text-slate-900 dark:hover:bg-slate-200">
              <Stop weight="fill" className="size-3.5" />
            </button>
          ) : (
            <button type="submit" disabled={!canSend} title={busy ? "Encolar (Enter)" : "Enviar (Enter)"} aria-label="Enviar"
              className="grid size-9 shrink-0 place-items-center rounded-full bg-slate-900 p-0 text-white transition hover:bg-slate-700 disabled:bg-slate-200 disabled:text-slate-400 dark:bg-white dark:text-slate-900 dark:hover:bg-slate-200 dark:disabled:bg-raised dark:disabled:text-muted">
              {send.isPending ? <span className="size-4 animate-spin rounded-full border-2 border-current border-t-transparent" /> : busy ? <ListBullets weight="bold" className="block size-4" /> : <ArrowUp weight="bold" className="block size-4" />}
            </button>
          )}
        </div>
        {(send.error || settings.error || stop.error) && <div className="px-4 pb-2"><ErrorText error={send.error ?? settings.error ?? stop.error} /></div>}
      </form>
    </div>
  );
}

// useFileSuggest opens a list of the thread's files after an @ and replaces
// the typed word with the chosen path.
function useFileSuggest(threadId: string, text: string, area: React.RefObject<HTMLTextAreaElement | null>, setText: (t: string) => void) {
  const [word, setWord] = useState<{ start: number; q: string } | null>(null);
  const [index, setIndex] = useState(0);
  const files = useQuery({ queryKey: ["thread-files", threadId, word?.q], queryFn: () => api.threadFiles(threadId, word!.q), enabled: !!word, staleTime: 30_000 });
  const list = word ? files.data ?? [] : [];
  const track = () => {
    const el = area.current;
    if (!el) return;
    const before = text.slice(0, el.selectionStart);
    const m = /(^|\s)@([^\s@]*)$/.exec(before);
    setWord(m ? { start: before.length - m[2].length - 1, q: m[2] } : null);
    setIndex(0);
  };
  useEffect(track, [text]); // eslint-disable-line react-hooks/exhaustive-deps
  const pick = (path: string) => {
    if (!word) return;
    const end = word.start + 1 + word.q.length;
    const next = `${text.slice(0, word.start)}@${path} ${text.slice(end)}`;
    setText(next);
    setWord(null);
    requestAnimationFrame(() => { const el = area.current; if (el) { const at = word.start + path.length + 2; el.focus(); el.setSelectionRange(at, at); } });
  };
  const onKeyDown = (e: React.KeyboardEvent) => {
    if (!list.length) return false;
    if (e.key === "ArrowDown" || e.key === "ArrowUp") { e.preventDefault(); setIndex((i) => (i + (e.key === "ArrowDown" ? 1 : list.length - 1)) % list.length); return true; }
    if (e.key === "Enter" || e.key === "Tab") { e.preventDefault(); pick(list[index]); return true; }
    if (e.key === "Escape") { e.preventDefault(); setWord(null); return true; }
    return false;
  };
  const menu = list.length > 0 && (
    <ul role="listbox" aria-label="Ficheros" className="absolute bottom-full left-3 z-30 mb-2 max-h-64 w-[min(28rem,calc(100%-1.5rem))] overflow-y-auto rounded-xl border border-slate-200 bg-white p-1 shadow-lg dark:border-border dark:bg-surface">
      {list.map((f, i) => (
        <li key={f}>
          <button type="button" role="option" aria-selected={i === index} onMouseDown={(e) => { e.preventDefault(); pick(f); }}
            className={`flex w-full items-center gap-2 rounded-lg px-2 py-1.5 text-left text-sm ${i === index ? "bg-slate-100 dark:bg-raised" : "hover:bg-slate-100 dark:hover:bg-raised"}`}>
            <FileIcon className="size-4 shrink-0 text-slate-400" />
            <span className="min-w-0 truncate"><span className="font-medium">{f.split("/").pop()}</span> <span className="text-xs text-slate-500 dark:text-muted">{f}</span></span>
          </button>
        </li>
      ))}
    </ul>
  );
  return { menu, track, onKeyDown };
}

// Queue shows the messages waiting for the running turn as outlined bubbles,
// each with its actions underneath.
export function Queue({ t }: { t: ThreadView }) {
  const qc = useQueryClient();
  const act = useMutation({
    mutationFn: ({ run, action, text }: { run: string; action: "edit" | "cancel" | "steer"; text?: string }) => api.queue(t.id, run, action, text),
    onSuccess: () => qc.invalidateQueries({ queryKey: ["thread", t.id] }),
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

function QueuedRow({ text, files, canSteer, onSteer, onCancel, onEdit }: { text: string; files: Attachment[]; canSteer: boolean; onSteer: () => void; onCancel: () => void; onEdit: (text: string) => void }) {
  const [draft, setDraft] = useState<string | null>(null);
  return (
    <div className="msg-in flex flex-col items-end gap-1">
      <div className="w-fit max-w-[85%] rounded-2xl rounded-br-md border border-dashed border-slate-400 px-4 py-2.5 text-slate-700 dark:border-border-strong dark:text-text-secondary">
        {draft === null ? <p className="whitespace-pre-wrap break-words text-sm">{text}</p> : (
          <form className="flex gap-1" onSubmit={(e) => { e.preventDefault(); onEdit(draft); setDraft(null); }}>
            <Input autoFocus value={draft} onChange={(e) => setDraft(e.target.value)} onKeyDown={(e) => { if (e.key === "Escape") setDraft(null); }} />
            <Button>Guardar</Button>
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
