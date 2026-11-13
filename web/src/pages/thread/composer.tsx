import { useEffect, useRef, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { ArrowUp, CaretDown, CaretRight, Check, FileText, Paperclip, Lightning, ListBullets, Play, Stop, X } from "@phosphor-icons/react";
import { api, type T3Attachment, type T3ThreadView } from "../../api/client";
import { ErrorText, errMsg, uuid } from "../../components/ui";
import { type QuestionControl } from "../../components/pending-request";
import { ProviderIcon } from "../launch-form";
import { saveEffort, savedEffort } from "../../effort";
import { PdfThumb } from "../../components/pdf-thumb";
import { useComposerSuggest } from "../../components/composer-suggest";
import { paintTokens, quoteBlock, tokenRanges, type Token, type Tokens } from "../../components/composer-tokens";
import { repoName } from "./header";
import { AttachmentPreview, Insert, Shown, isPdf } from "./chat";

export const runtimeModes = [
  { value: "approval-required", label: "Pedir permiso" },
  { value: "auto-accept-edits", label: "Aceptar ediciones" },
  { value: "auto", label: "Automático" },
  { value: "full-access", label: "Acceso total" },
];

export type MenuOption = { value: string; label: string; group?: string; icon?: React.ReactNode };

// Chip is a compact dropdown for the composer toolbar; the menu opens upwards.
export function Chip({ value, options, onChange, title, icon }: { value: string; options: MenuOption[]; onChange: (v: string) => void; title: string; icon?: React.ReactNode }) {
  const [open, setOpen] = useState(false);
  const box = useRef<HTMLDivElement>(null);
  useEffect(() => {
    if (!open) return;
    const close = (e: PointerEvent) => { if (!box.current?.contains(e.target as Node)) setOpen(false); };
    document.addEventListener("pointerdown", close);
    return () => document.removeEventListener("pointerdown", close);
  }, [open]);
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
          {options.map((o, i) => (
            <li key={o.value}>
              {o.group && o.group !== options[i - 1]?.group && <p className="px-2 pb-1 pt-2 text-[11px] font-semibold uppercase tracking-wide text-slate-400">{o.group}</p>}
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

// ModelPicker joins model and effort in one chip; its panel lists models on
// the left and, on the right, the efforts of the model last clicked. Picking a
// model preselects the effort last used with it.
export function ModelPicker({ models, selected, effort, effortsOf, onChange }: { models: MenuOption[]; selected: string; effort: string; effortsOf: (value: string) => { values: string[]; initial: string } | undefined; onChange: (value: string, effort: string) => void }) {
  const [open, setOpen] = useState(false);
  const [focus, setFocus] = useState(selected);
  const box = useRef<HTMLDivElement>(null);
  useEffect(() => {
    if (!open) return;
    setFocus(selected);
    const close = (e: PointerEvent) => { if (!box.current?.contains(e.target as Node)) setOpen(false); };
    document.addEventListener("pointerdown", close);
    return () => document.removeEventListener("pointerdown", close);
  }, [open, selected]);
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
            {models.map((o, i) => (
              <li key={o.value}>
                {o.group && o.group !== models[i - 1]?.group && <p className="px-2 pb-1 pt-2 text-[11px] font-semibold uppercase tracking-wide text-slate-400">{o.group}</p>}
                <button type="button" onClick={() => pickModel(o.value)}
                  className={`${row} ${o.value === focus ? "bg-slate-100 dark:bg-raised" : ""}`}>
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

export type Draft = { key: string; name: string; mime: string; preview: string; done?: T3Attachment; error?: string };

export function Composer({ v, queryKey, onStop, question, beforeSend, insert }: { v: T3ThreadView; queryKey: unknown[]; onStop: (run: string) => void; question: QuestionControl | null; beforeSend?: () => Promise<unknown>; insert?: Insert }) {
  const qc = useQueryClient();
  const catalog = useQuery({ queryKey: ["t3-catalog", v.instance_id], queryFn: () => api.t3Catalog(v.instance_id), enabled: !!v.instance_id, staleTime: 60_000 });
  const [text, setText] = useState("");
  const [choice, setChoice] = useState("");
  const [effort, setEffort] = useState("");
  const area = useRef<HTMLTextAreaElement>(null);
  const picker = useRef<HTMLInputElement>(null);
  const [files, setFiles] = useState<Draft[]>([]);
  const uploading = files.some((f) => !f.done && !f.error);
  const ready = files.flatMap((f) => (f.done ? [f.done] : []));
  // Each file uploads as soon as it is added; the message carries the results.
  const addFiles = (list: FileList | File[]) => {
    for (const file of Array.from(list)) {
      const key = uuid(); // crypto.randomUUID needs HTTPS, the dev preview is plain HTTP
      const named = file.name ? file : new File([file], `imagen-${new Date().toISOString().slice(11, 19).replace(/:/g, "")}.png`, { type: file.type });
      const preview = URL.createObjectURL(named); // also opens PDFs and text in the preview
      setFiles((fs) => [...fs, { key, name: named.name, mime: named.type, preview }]);
      api.t3Upload(v.id, named).then(
        (done) => setFiles((fs) => fs.map((f) => (f.key === key ? { ...f, done } : f))),
        (e: unknown) => setFiles((fs) => fs.map((f) => (f.key === key ? { ...f, error: errMsg(e) || "No se pudo subir" } : f))),
      );
    }
  };
  const [viewing, setViewing] = useState<Shown | null>(null);
  const removeFile = (key: string) => setFiles((fs) => fs.filter((f) => { if (f.key === key) URL.revokeObjectURL(f.preview); return f.key !== key; }));
  const selected = choice || `${v.provider_id}|${v.model}`;
  const [providerId, slug] = selected.split("|");
  // / commands and $ skills come from the provider that will run the message.
  const prov = catalog.data?.providers.find((p) => p.instance_id === providerId);
  // The repository's own commands and skills (T3 scans the thread folder) load in the background;
  // until then, and if the scan fails, the provider's global ones from the catalog are used.
  const composer = useQuery({ queryKey: ["t3-composer", v.id, providerId], queryFn: () => api.t3Composer(v.id, providerId), enabled: !!providerId, staleTime: 5 * 60_000, retry: false });
  const projectRepo = repoName(catalog.data?.projects.find((p) => p.id === v.project_id)?.repo);
  // Picked /commands, $skills, @files and #PRs live in the text as tokens: the
  // text holds their characters, tokens their reference (a file's full path).
  const [tokens, setTokens] = useState<Tokens>({});
  const addToken = (insert: string, t: Token) => {
    // Two files with the same name: the second one is written with its path.
    const key = t.kind === "path" && tokens[insert] && tokens[insert].value !== t.value ? `@${t.value}` : insert;
    setTokens((ts) => ({ ...ts, [key]: t }));
    return key;
  };
  useEffect(() => {
    if (!insert) return;
    let add = insert.text;
    if (insert.quote) {
      // The quote shows as one object; a repeated label gets a number.
      let key = `«${insert.quote.label}»`;
      for (let n = 2; tokens[key] && tokens[key].value !== insert.quote.body; n++) key = `«${insert.quote.label} (${n})»`;
      setTokens((ts) => ({ ...ts, [key]: { kind: "quote", value: insert.quote!.body, title: insert.quote!.label } }));
      add = `${key} ${insert.text}`;
    }
    setText((t) => (t.trim() ? `${t.trimEnd()}\n${add}` : add));
    setTimeout(() => { const el = area.current; if (el) { el.focus(); el.setSelectionRange(el.value.length, el.value.length); } }, 0);
  }, [insert?.seq]); // eslint-disable-line react-hooks/exhaustive-deps
  const ranges = tokenRanges(text, tokens);
  const present = ranges.map((r) => tokens[r.text]);
  const mentions = [...new Set(present.filter((t) => t.kind === "path").map((t) => t.value))];
  const skillsUsed = [...new Set(present.filter((t) => t.kind === "skill").map((t) => t.value))];
  // T3 gets each @file with its full path; the other tokens go as written.
  // T3 gets each @file with its full path and each quote as a block quote; the other tokens go as written.
  const outgoing = ranges.reduceRight((t, r) => {
    const tk = tokens[r.text];
    if (tk.kind === "path") return `${t.slice(0, r.start)}@${tk.value}${t.slice(r.end)}`;
    if (tk.kind === "quote") return `${t.slice(0, r.start)}${quoteBlock(tk.title ?? "", tk.value)}${t.slice(r.end).replace(/^ /, "")}`;
    return t;
  }, text).trim();
  const paint = useRef<HTMLDivElement>(null);
  // A token deletes as a whole: Backspace at its end or inside, Delete at its start or inside.
  // A selection never splits a token: an edge inside one grows to cover it, and
  // a caret inside one jumps past it in the direction it was moving (arrows
  // step over a token in one press), or to the nearest edge on a click.
  const lastCaret = useRef(0);
  const snapSelection = () => {
    const el = area.current;
    if (!el) return;
    const { selectionStart: s0, selectionEnd: e0, selectionDirection: dir } = el;
    let s = s0, en = e0;
    for (const r of ranges) {
      if (s0 === e0) {
        if (s0 > r.start && s0 < r.end) {
          const prev = lastCaret.current;
          s = en = prev <= r.start ? r.end : prev >= r.end ? r.start : s0 - r.start < r.end - s0 ? r.start : r.end;
        }
      } else {
        if (s > r.start && s < r.end) s = r.start;
        if (en > r.start && en < r.end) en = r.end;
      }
    }
    if (s !== s0 || en !== e0) el.setSelectionRange(s, en, dir ?? undefined);
    lastCaret.current = s === en ? s : dir === "backward" ? s : en;
  };
  const deleteToken = (e: React.KeyboardEvent<HTMLTextAreaElement>) => {
    const el = e.currentTarget;
    if ((e.key !== "Backspace" && e.key !== "Delete") || el.selectionStart !== el.selectionEnd) return false;
    const at = el.selectionStart;
    // Ctrl/Alt/Cmd delete a word: across the spaces up to a token, the token is the word.
    const word = e.ctrlKey || e.altKey || e.metaKey;
    const reach = (r: { start: number; end: number }) => e.key === "Backspace"
      ? (at > r.start && at <= r.end) || (word && at > r.end && !text.slice(r.end, at).trim())
      : (at >= r.start && at < r.end) || (word && at < r.start && !text.slice(at, r.start).trim());
    const hit = ranges.find(reach);
    if (!hit) return false;
    e.preventDefault();
    // A word delete also takes the spaces between the caret and the token.
    const from = e.key === "Backspace" ? hit.start : Math.min(at, hit.start);
    const to = e.key === "Backspace" ? Math.max(at, hit.end) : hit.end;
    setText(text.slice(0, from) + text.slice(to));
    requestAnimationFrame(() => el.setSelectionRange(from, from));
    return true;
  };
  const suggest = useComposerSuggest({ threadId: v.id, text, setText, area, commands: composer.data?.commands ?? prov?.commands ?? [], skills: composer.data?.skills ?? prov?.skills ?? [], repo: projectRepo, onToken: addToken });
  const model = catalog.data?.providers.find((p) => p.instance_id === providerId)?.models.find((m) => m.slug === slug);
  const effortValue = effort || (choice ? (model?.effort ? savedEffort(model.slug, model.effort.values) || model.effort.default : "") : v.effort);
  const changed = !!choice || !!effort;

  // Grow the textarea with its content, up to about ten lines.
  useEffect(() => {
    const el = area.current;
    if (!el) return;
    el.style.height = "auto";
    el.style.height = `${Math.min(el.scrollHeight, 240)}px`;
  }, [text]);

  const send = useMutation({
    // While a turn runs, Ctrl/Cmd+Enter steers and Enter (or the button) queues.
    mutationFn: async (asSteer?: boolean) => { await beforeSend?.(); return api.t3Dispatch(v.id, {
      // Inline @labels go out as @full/path (what the agent reads) and as context records.
      text: outgoing,
      mode: asSteer ? "steer" : "auto", ...(ready.length ? { attachments: ready } : {}), skills: skillsUsed, mentions,
      ...(changed ? { provider_id: providerId, model: slug, effort_option: model?.effort?.id, effort: model?.effort ? effortValue : undefined } : {}),
    }); },
    onSuccess: () => { if (changed && model?.effort && effortValue) saveEffort(model.slug, effortValue); setText(""); setTokens({}); suggest.reset(); files.forEach((f) => URL.revokeObjectURL(f.preview)); setFiles([]); qc.invalidateQueries({ queryKey }); },
  });
  const settings = useMutation({ mutationFn: (b: { interaction_mode?: string; runtime_mode?: string }) => api.t3Settings(v.id, b), onSuccess: () => qc.invalidateQueries({ queryKey }) });
  const busy = !!v.active_run_id;
  const models: MenuOption[] = (catalog.data?.providers ?? []).flatMap((p) => p.models.map((m) => ({ value: `${p.instance_id}|${m.slug}`, label: m.name, group: p.name, icon: <ProviderIcon driver={p.driver || p.name} model={`${m.slug} ${m.name}`} /> })));
  if (!models.some((m) => m.value === selected)) models.unshift({ value: selected, label: v.model || "Modelo del hilo", icon: <ProviderIcon driver={v.provider_id} model={v.model} /> });
  // Optimistic: the thread shows stopped at once; an error brings the run back.
  const stop = useMutation({
    mutationFn: () => api.t3Interrupt(v.id),
    onMutate: () => onStop(v.active_run_id),
    onError: () => onStop(""),
    onSettled: () => qc.invalidateQueries({ queryKey }),
  });
  const held = !busy && v.queue.some((r) => r.held);
  // Continue picks up where the stopped turn left off: the held queue first, else a nudge.
  const resume = useMutation({
    mutationFn: () => held ? api.t3QueueResume(v.id) : api.t3Dispatch(v.id, { text: "Continúa por donde lo dejaste.", mode: "auto" }),
    onSuccess: () => qc.invalidateQueries({ queryKey }),
  });
  // Stopped with an empty box: the send button continues where it stopped.
  const resumable = !busy && (held || v.interrupted) && !text.trim() && !files.length;
  const canSend = (!!text.trim() || ready.length > 0 || resumable) && !uploading && !send.isPending && !resume.isPending;
  const canNext = !!question && !question.busy && (!!text.trim() || question.picked);
  const submit = (asSteer?: boolean) => {
    // An open question takes the typed text as its free answer, or its checked options.
    if (question) { if (canNext) { question.next(text.trim()); setText(""); } return; }
    if (!canSend) return;
    if (resumable) resume.mutate(); else send.mutate(busy ? asSteer : undefined);
  };

  return (
    <form onSubmit={(e) => { e.preventDefault(); submit(); }}
      onDragOver={(e) => { if (e.dataTransfer.types.includes("Files")) e.preventDefault(); }}
      onDrop={(e) => { if (e.dataTransfer.files.length) { e.preventDefault(); addFiles(e.dataTransfer.files); } }}
      onPaste={(e) => {
        // Screenshots often come only as clipboard items, not in .files.
        const fs = Array.from(e.clipboardData.items).flatMap((i) => { const f = i.kind === "file" ? i.getAsFile() : null; return f ? [f] : []; });
        if (fs.length) { e.preventDefault(); addFiles(fs); }
      }}
      className="relative rounded-3xl border border-slate-200 bg-white shadow-md dark:border-border-strong dark:bg-surface">
      {suggest.menu}
      {files.length > 0 && (
        <div className="flex flex-wrap gap-2 px-3 pt-3">
          {files.map((f) => (
            <div key={f.key} title={f.error ?? f.name} className={`group relative size-14 shrink-0 overflow-hidden rounded-xl border bg-slate-50 dark:bg-raised ${f.error ? "border-red-400" : "border-slate-200 dark:border-border"}`}>
              <button type="button" onClick={() => setViewing({ src: f.preview, name: f.name, mime: f.mime })} aria-label={`Ver ${f.name}`} className="block size-14 cursor-zoom-in">
                {f.mime.startsWith("image/") ? <img src={f.preview} alt="" className="size-14 shrink-0 object-cover" /> : isPdf(f) ? <PdfThumb src={f.preview} width={56} className="size-14" /> : <span className="grid size-14 shrink-0 place-items-center text-slate-400"><FileText className="size-6" /></span>}
              </button>
              {f.error && <span className="absolute inset-x-0 bottom-0 bg-red-500 text-center text-[9px] font-medium text-white">Error</span>}
              {!f.done && !f.error && <span className="absolute inset-0 grid place-items-center bg-white/60 dark:bg-black/40"><span className="size-4 animate-spin rounded-full border-2 border-current border-t-transparent" /></span>}
              <button type="button" onClick={() => removeFile(f.key)} aria-label={`Quitar ${f.name}`} className="absolute right-1 top-1 grid size-5 place-items-center rounded-full bg-slate-900/70 text-white transition-opacity hover:bg-slate-900 sm:opacity-0 sm:group-hover:opacity-100 sm:focus-visible:opacity-100"><X weight="regular" className="size-3" /></button>
            </div>
          ))}
        </div>
      )}
      {viewing && <AttachmentPreview {...viewing} onClose={() => setViewing(null)} />}
      <div className="relative">
        {/* Inline @file chips: this layer paints the text under a see-through textarea, glyph for glyph, so the caret stays aligned. */}
        {ranges.length > 0 && <div ref={paint} aria-hidden="true" className="pointer-events-none absolute inset-0 overflow-hidden whitespace-pre-wrap break-words px-4 pb-1 pt-3.5 text-[15px] leading-6">
          {paintTokens(text, tokens)}{"\u200b"}
        </div>}
        <textarea ref={area} rows={1} value={text} onChange={(e) => { setText(e.target.value); suggest.track(); }} onSelect={() => { snapSelection(); suggest.track(); }} onClick={suggest.track} aria-label="Mensaje"
          onScroll={(e) => { if (paint.current) paint.current.scrollTop = e.currentTarget.scrollTop; }}
          placeholder={question ? "Escribe tu respuesta a la pregunta…" : "Escribe al hilo…"}
          onKeyDown={(e) => { if (e.nativeEvent.isComposing || suggest.onKeyDown(e) || deleteToken(e)) return; if (e.key === "Enter" && !e.shiftKey) { e.preventDefault(); submit(e.ctrlKey || e.metaKey); } }}
          className={`relative block max-h-60 w-full resize-none break-words bg-transparent px-4 pb-1 pt-3.5 text-[15px] leading-6 !outline-none placeholder:text-slate-400 ${ranges.length ? "text-transparent caret-slate-900 dark:caret-white" : ""}`} />
      </div>
      <div className="flex items-center gap-0.5 px-2 pb-2">
        <div className="flex min-w-0 flex-1 flex-wrap items-center gap-0.5">
          <input ref={picker} type="file" multiple hidden onChange={(e) => { if (e.target.files) addFiles(e.target.files); e.target.value = ""; }} />
          <button type="button" onClick={() => picker.current?.click()} title="Adjuntar imágenes o ficheros" aria-label="Adjuntar"
            className="grid size-8 place-items-center rounded-full text-slate-500 hover:bg-slate-100 hover:text-slate-900 dark:text-text-secondary dark:hover:bg-raised dark:hover:text-white">
            <Paperclip weight="regular" className="size-4" />
          </button>
          <ModelPicker models={models} selected={selected} effort={effortValue}
            effortsOf={(value) => { const [pid, sl] = value.split("|"); const m = catalog.data?.providers.find((p) => p.instance_id === pid)?.models.find((x) => x.slug === sl); return m?.effort ? { values: m.effort.values, initial: value === selected ? effortValue : savedEffort(sl, m.effort.values) || m.effort.default } : undefined; }}
            onChange={(value, eff) => { setChoice(value); setEffort(eff); const sl = value.split("|")[1]; if (eff) saveEffort(sl, eff); }} />
          <Chip title="Permisos" value={v.runtime_mode} options={runtimeModes} onChange={(x) => settings.mutate({ runtime_mode: x })} />
        </div>
        {question ? (
          <>
            {busy && (
              <button type="button" onClick={() => stop.mutate()} disabled={stop.isPending} title="Parar el turno en curso" aria-label="Parar"
                className="grid size-9 shrink-0 place-items-center rounded-full border border-slate-200 text-slate-700 transition hover:bg-slate-100 disabled:opacity-50 dark:border-border dark:text-text-secondary dark:hover:bg-raised">
                <Stop weight="fill" className="size-3.5" />
              </button>
            )}
            <button type="submit" disabled={!canNext} title={question.last ? "Responder (Enter)" : "Siguiente (Enter)"}
              className="ml-1 inline-flex h-9 shrink-0 items-center gap-1 rounded-full bg-slate-900 px-4 text-sm font-medium text-white transition hover:bg-slate-700 disabled:bg-slate-200 disabled:text-slate-400 dark:bg-white dark:text-slate-900 dark:hover:bg-slate-200 dark:disabled:bg-raised dark:disabled:text-muted">
              {question.last ? "Responder" : "Siguiente"}<CaretRight weight="regular" className="size-3.5" />
            </button>
          </>
        ) : busy && !text.trim() ? (
          <button type="button" onClick={() => stop.mutate()} disabled={stop.isPending} title="Parar el turno en curso" aria-label="Parar"
            className="grid size-9 shrink-0 place-items-center rounded-full bg-slate-900 text-white transition hover:bg-slate-700 disabled:opacity-50 dark:bg-white dark:text-slate-900 dark:hover:bg-slate-200">
            <Stop weight="fill" className="size-3.5" />
          </button>
        ) : <>{busy && <span className="mr-2 hidden items-center gap-1 text-[11px] text-slate-400 dark:text-muted sm:inline-flex" title="Ctrl+Enter inyecta el mensaje en el turno en curso; Enter lo pone en cola"><Lightning className="size-3" aria-hidden="true" /><kbd className="font-sans">Ctrl+Enter</kbd></span>}<button type="submit" disabled={!canSend} title={resumable ? "Continuar por donde iba" : busy ? "Encolar (Enter) · Ctrl+Enter: steer" : "Enviar (Enter)"} aria-label={resumable ? "Continuar" : "Enviar"}
          className="grid size-9 shrink-0 place-items-center rounded-full bg-slate-900 p-0 text-white transition hover:bg-slate-700 disabled:bg-slate-200 disabled:text-slate-400 dark:bg-white dark:text-slate-900 dark:hover:bg-slate-200 dark:disabled:bg-raised dark:disabled:text-muted">
          {send.isPending || resume.isPending ? <span className="size-4 animate-spin rounded-full border-2 border-current border-t-transparent" /> : resumable ? <Play weight="fill" className="block size-3.5 translate-x-px" /> : busy ? <ListBullets weight="bold" className="block size-4" /> : <ArrowUp weight="bold" className="block size-4" />}
        </button></>}
      </div>
      {(send.error || settings.error || stop.error || resume.error) && <div className="px-4 pb-2"><ErrorText error={send.error ?? settings.error ?? stop.error ?? resume.error} /></div>}
    </form>
  );
}
