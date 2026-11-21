import { useEffect, useId, useLayoutEffect, useRef, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { ArrowUp, CaretDown, Check, ClipboardText, Code, File as FileIcon, FileText, Lightning, ListBullets, MagnifyingGlass, Paperclip, PencilSimple, Quotes, Stop, TerminalWindow, Trash, X } from "@phosphor-icons/react";
import { api, usable, type Attachment, type ThreadView } from "../api/client";
import { Button, ErrorText, errMsg, iconButton, uuid } from "../components/ui";
import { DictationField } from "../components/dictation-field";
import { DictationMic, DictationPreview, useDictationInProgress, useLocalDictation } from "../components/local-dictation";
import { claudeModelName } from "../components/model-label";
import { ProviderIcon } from "../components/launch-picker";
import { explicitModels, findModel, modelEffort, modelId as concreteModelId, rememberModel } from "../effort";
import { iconBtn, MessageParts } from "./chat";
import { HoverCard } from "../components/tooltip";
import { CodeRefCard, codeRefText } from "../components/code-ref";
import type { DraftQuote } from "../components/quote-selection";
import { askAnswerText, openAsk, QuestionPanel, useQuestionFlow } from "../components/question-card";
import { useCommand, useShortcuts } from "../components/shortcuts";
import { formatChord } from "../components/dictation-shortcut";
import { autoMessage } from "../lib/auto-message";
import { paintTokens, quoteBlock, quoteLabel, tokenRanges, type Token, type Tokens } from "../components/composer-tokens";

// The composer of a thread, kept from Dory v1: attachments, @ mentions of
// files, model, effort and permissions, and send, queue or stop.

export const composerFrame = "relative z-10 rounded-3xl border border-slate-200 bg-white shadow-md dark:border-raised dark:bg-surface";
export const composerArea = "relative block max-h-60 w-full resize-none break-words bg-transparent px-4 pb-1 pt-3.5 text-[0.9375rem] leading-6 !outline-none placeholder:text-slate-400";

export type MenuOption = { value: string; label: string; group?: string; modelId?: string; icon?: React.ReactNode };

// Chip is a compact dropdown for the composer toolbar; the menu opens upwards.
export function Chip({ value, options, onChange, title, icon, below = false, prefix, shortcutBranch = false }: { value: string; options: MenuOption[]; onChange: (v: string) => void; title: string; icon?: React.ReactNode; below?: boolean; prefix?: string; shortcutBranch?: boolean }) {
  const [open, setOpen] = useState(false);
  const getShortcut = useShortcuts();
  const box = useOutside(open, () => setOpen(false));
  const current = options.find((o) => o.value === value);
  const trigger = useRef<HTMLButtonElement>(null);
  const [query, setQuery] = useState("");
  const [highlight, setHighlight] = useState(0);
  const filtered = options.filter((o) => o.label.toLowerCase().includes(query.toLowerCase()));
  const close = () => { setOpen(false); setQuery(""); trigger.current?.focus(); };
  useCommand("branch", () => { if (!trigger.current?.getClientRects().length) return false; setQuery(""); setHighlight(0); setOpen(true); }, shortcutBranch);
  return (
    <div ref={box} className="relative">
      <button ref={trigger} type="button" title={shortcutBranch ? `${title} (${formatChord(getShortcut("branch"))})` : title} aria-haspopup="listbox" aria-expanded={open} onClick={() => setOpen(!open)}
        onKeyDown={(e) => { if (e.key === "Escape") setOpen(false); }}
        className="inline-flex h-8 max-w-[11rem] items-center gap-1.5 rounded-md px-2.5 text-xs font-medium text-slate-600 transition-colors hover:bg-slate-100 hover:text-slate-900 dark:text-text-secondary dark:hover:bg-raised dark:hover:text-white">
        {current?.icon ?? icon}<span className="truncate">{prefix && `${prefix} `}{current?.label ?? value}</span><CaretDown weight="regular" className="size-3 shrink-0 opacity-60" />
      </button>
      {open && (
        <ul role="listbox" className={`absolute left-0 z-20 max-h-72 min-w-[12rem] overflow-y-auto rounded-xl border border-slate-200 bg-white p-1 shadow-lg dark:border-border dark:bg-surface ${below ? "top-full mt-2" : "bottom-full mb-2"}`}>
          {shortcutBranch && <li><input autoFocus aria-label="Buscar rama" value={query} onChange={(e) => { setQuery(e.target.value); setHighlight(0); }} onKeyDown={(e) => {
            if (e.key === "Escape") { e.stopPropagation(); close(); }
            else if (e.key === "ArrowDown") { e.preventDefault(); setHighlight((i) => Math.min(i + 1, filtered.length - 1)); }
            else if (e.key === "ArrowUp") { e.preventDefault(); setHighlight((i) => Math.max(i - 1, 0)); }
            else if (e.key === "Enter" && filtered[highlight]) { e.preventDefault(); onChange(filtered[highlight].value); close(); }
          }} className="w-full rounded bg-transparent px-2 py-1.5 text-sm outline-none focus:ring-2 focus:ring-blue-500" /></li>}
          {filtered.map((o, i) => (
            <li key={o.value}>
              <button type="button" role="option" aria-selected={o.value === value} onClick={() => { onChange(o.value); close(); }}
                className={`flex w-full items-center gap-2 rounded-lg px-2 py-1.5 text-left text-sm hover:bg-slate-100 dark:hover:bg-raised ${shortcutBranch && i === highlight ? "bg-slate-100 dark:bg-raised" : ""}`}>
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

type ModelOption = MenuOption & { provider: string };
type ModelProvider = { id: string; name: string; disabled?: boolean };

// ModelPicker filters the active provider's catalog and shows the selected model's reasoning levels.
export function ModelPicker({ models, providers, selected, effort, effortsOf, onChange, modelId = selected, reportedEffort }: { models: ModelOption[]; providers: ModelProvider[]; selected: string; effort: string; effortsOf: (value: string) => { values: string[]; initial: string } | undefined; onChange: (value: string, effort: string) => void; modelId?: string; reportedEffort?: string }) {
  const [open, setOpen] = useState(false);
  const getShortcut = useShortcuts();
  const [focus, setFocus] = useState(selected);
  const [provider, setProvider] = useState("");
  const [search, setSearch] = useState("");
  const [navigationIndex, setNavigationIndex] = useState<number>();
  const trigger = useRef<HTMLButtonElement>(null);
  const panel = useRef<HTMLDivElement>(null);
  const [placement, setPlacement] = useState<{ above: boolean; maxHeight: number }>();
  const gap = 8;
  const modelList = useRef<HTMLUListElement>(null);
  const listId = useId();
  const box = useOutside(open, () => setOpen(false));
  useLayoutEffect(() => {
    const button = trigger.current;
    const menu = panel.current;
    if (!open || !button || !menu) return;

    const viewport = window.visualViewport;
    const place = () => {
      const anchor = button.getBoundingClientRect();
      const viewportTop = viewport?.offsetTop ?? 0;
      const viewportBottom = viewportTop + (viewport?.height ?? window.innerHeight);
      const aboveSpace = anchor.top - viewportTop - 2 * gap;
      const belowSpace = viewportBottom - anchor.bottom - 2 * gap;
      const menuHeight = menu.scrollHeight + (menu.offsetHeight - menu.clientHeight);
      const above = menuHeight <= aboveSpace || (menuHeight > belowSpace && aboveSpace > belowSpace);
      const maxHeight = Math.max(0, Math.floor(above ? aboveSpace : belowSpace));
      setPlacement((previous) => previous?.above === above && previous.maxHeight === maxHeight ? previous : { above, maxHeight });
    };
    place();
    const resize = new ResizeObserver(place);
    resize.observe(menu);
    resize.observe(button);
    window.addEventListener("resize", place);
    window.addEventListener("scroll", place, true);
    viewport?.addEventListener("resize", place);
    viewport?.addEventListener("scroll", place);
    return () => {
      resize.disconnect();
      window.removeEventListener("resize", place);
      window.removeEventListener("scroll", place, true);
      viewport?.removeEventListener("resize", place);
      viewport?.removeEventListener("scroll", place);
    };
  }, [open]);
  const current = models.find((m) => m.value === selected);
  const activeProvider = providers.find((item) => item.id === provider && !item.disabled) ?? providers.find((item) => !item.disabled);
  const query = search.trim().toLowerCase();
  const visibleModels = models.filter((model) =>
    model.provider === activeProvider?.id &&
    [model.label, model.modelId ?? model.value].some((name) => name.toLowerCase().includes(query)),
  );
  const highlightedIndex = navigationIndex ?? (query ? 0 : undefined);
  const highlightedModel = highlightedIndex === undefined ? undefined : visibleModels[highlightedIndex];
  const close = () => {
    setOpen(false);
    trigger.current?.focus();
  };
  const toggle = () => {
    if (open) {
      close();
      return;
    }
    setProvider(current?.provider ?? activeProvider?.id ?? "");
    setFocus(selected);
    setSearch("");
    setNavigationIndex(undefined);
    setOpen(true);
  };
  useCommand("model", () => { if (!trigger.current?.getClientRects().length) return false; toggle(); });
  const claude = claudeModelName(modelId);
  const modelLabel = claude ||
    (modelId === (current?.modelId ?? selected) ? current?.label : undefined) ||
    (modelId && modelId !== "default" ? modelId : "Seleccionar modelo");
  const effortLabel = reportedEffort || effort;
  const settingsLabel = effortLabel ? `${modelLabel} · ${effortLabel}` : modelLabel;
  const efforts = visibleModels.some((model) => model.value === focus) ? effortsOf(focus) : undefined;
  const pickModel = (value: string, closeAfterSelection = false) => {
    const e = effortsOf(value);
    setFocus(value);
    onChange(value, value === selected && effort ? effort : e?.initial ?? "");
    if (!e || closeAfterSelection) close();
  };
  const row = "flex w-full items-center gap-2 rounded-lg px-2 py-1.5 text-left text-sm hover:bg-slate-100 dark:hover:bg-raised";
  return (
    <div ref={box} className="relative flex">
      <button ref={trigger} type="button" title={`${settingsLabel}${claude ? ` (${modelId})` : ""} · ${formatChord(getShortcut("model"))}`} aria-label={`Modelo y razonamiento: ${settingsLabel}`} aria-haspopup="dialog" aria-expanded={open} onClick={toggle}
        className="inline-flex h-8 max-w-[20rem] items-center gap-1.5 rounded-md px-2.5 text-xs font-medium text-slate-600 transition-colors hover:bg-slate-100 hover:text-slate-900 dark:text-text-secondary dark:hover:bg-raised dark:hover:text-white">
        {current?.icon}<span className="truncate text-slate-900 dark:text-white">{modelLabel}</span>
        {effortLabel && <span className="shrink-0 text-slate-500 dark:text-muted">· {effortLabel}</span>}<CaretDown weight="regular" className="size-3 shrink-0 opacity-60" />
      </button>
      {open && (
        <div ref={panel} role="dialog" aria-label="Modelo y razonamiento"
          onKeyDown={(e) => { if (e.key === "Escape" && !e.nativeEvent.isComposing) { e.stopPropagation(); close(); } }}
          style={{
            [placement?.above === false ? "top" : "bottom"]: `calc(100% + ${gap}px)`,
            maxHeight: placement?.maxHeight,
          }}
          className="absolute left-0 z-20 w-[min(36rem,calc(100vw-4.5rem))] overflow-y-auto rounded-xl border border-slate-200 bg-white shadow-lg dark:border-border dark:bg-surface">
          <div className="flex items-center gap-2 border-b border-slate-200 px-3 dark:border-border">
            <MagnifyingGlass weight="regular" className="size-4 shrink-0 text-slate-500 dark:text-muted" />
            <input autoFocus type="search" role="combobox" aria-expanded={open} aria-controls={listId} aria-autocomplete="list"
              aria-activedescendant={highlightedModel ? `${listId}-${highlightedIndex}` : undefined}
              value={search} placeholder="Buscar modelos…" aria-label="Buscar modelos"
              onChange={(e) => {
                setSearch(e.currentTarget.value);
                setNavigationIndex(undefined);
                if (modelList.current) modelList.current.scrollTop = 0;
              }}
              onKeyDown={(e) => {
                if (e.nativeEvent.isComposing) return;
                if (e.key === "Enter") {
                  e.preventDefault();
                  if (highlightedModel) pickModel(highlightedModel.value, true);
                  return;
                }
                if (e.key !== "ArrowDown" && e.key !== "ArrowUp") return;
                e.preventDefault();
                if (!visibleModels.length) return;
                const lastIndex = visibleModels.length - 1;
                const down = e.key === "ArrowDown";
                const next = highlightedModel && highlightedIndex !== undefined
                  ? Math.max(0, Math.min(lastIndex, highlightedIndex + (down ? 1 : -1)))
                  : down ? 0 : lastIndex;
                setNavigationIndex(next);
                modelList.current?.children.item(next)?.scrollIntoView({ block: "nearest" });
              }}
              className="min-w-0 flex-1 appearance-none border-0 bg-transparent py-2.5 text-sm outline-none! focus-visible:outline-none!" />
          </div>
          <div className="grid grid-cols-[minmax(4.75rem,0.7fr)_minmax(0,2fr)] sm:grid-cols-[8.5rem_minmax(0,1fr)_7rem]">
            <ul aria-label="Proveedores" className="row-span-2 max-h-64 overflow-y-auto border-r border-slate-200 p-1 dark:border-border sm:row-span-1">
              {providers.map((item) => (
                <li key={item.id}>
                  <button type="button" disabled={item.disabled} aria-pressed={activeProvider?.id === item.id}
                    title={item.disabled ? "Espera a que termine el turno para cambiar de proveedor" : item.name}
                    onClick={() => {
                      setProvider(item.id);
                      setNavigationIndex(undefined);
                      if (modelList.current) modelList.current.scrollTop = 0;
                    }}
                    className={`${row} disabled:opacity-50`}>
                    <ProviderIcon driver={item.id} />
                    <span className="min-w-0 flex-1 truncate">{item.name}</span>
                    {activeProvider?.id === item.id && <Check weight="thin" className="size-3.5 shrink-0" />}
                  </button>
                </li>
              ))}
            </ul>
            <ul ref={modelList} id={listId} role="listbox" aria-label="Modelos" className="max-h-44 min-w-0 overflow-y-auto p-1 sm:max-h-64">
              {visibleModels.map((model, index) => (
                <li key={model.value} role="presentation">
                  <button id={`${listId}-${index}`} type="button" role="option" onClick={() => pickModel(model.value)} title={model.label}
                    aria-selected={model.value === selected}
                    className={`${row} ${model === highlightedModel ? "ring-1 ring-inset ring-slate-500 dark:ring-accent-text" : ""}`}>
                    {model.icon}<span className="min-w-0 flex-1 truncate">{model.label}</span>
                    {model.value === selected && <Check weight="thin" className="size-3.5 shrink-0" />}
                  </button>
                </li>
              ))}
              {!visibleModels.length && <li role="presentation" className="px-2 py-1.5 text-sm text-slate-500 dark:text-muted">{search.trim() ? "Sin resultados" : "Sin modelos disponibles"}</li>}
            </ul>
            {efforts && (
              <div aria-label="Razonamiento" className="flex flex-wrap gap-1 border-t border-slate-200 p-1 dark:border-border sm:block sm:max-h-64 sm:overflow-y-auto sm:border-l sm:border-t-0">
                {efforts.values.map((level) => {
                  const active = level === (focus === selected ? effort : efforts.initial);
                  return (
                    <button key={level} type="button" aria-pressed={active} onClick={() => { onChange(focus, level); close(); }}
                      className={`${row} !w-auto sm:!w-full`}>
                      <span className="min-w-0 flex-1 truncate">{level}</span>{active && <Check weight="thin" className="size-3.5 shrink-0" />}
                    </button>
                  );
                })}
              </div>
            )}
          </div>
        </div>
      )}
    </div>
  );
}

type Draft = { key: string; name: string; mime: string; preview: string; done?: Attachment; error?: string };

export function Composer({ t, onSent, area, quotes, onRemoveQuotes }: {
  t: ThreadView;
  onSent: () => void;
  area: React.RefObject<HTMLTextAreaElement | null>;
  quotes: DraftQuote[];
  onRemoveQuotes: (ids: string[]) => void;
}) {
  const qc = useQueryClient();
  const getShortcut = useShortcuts();
  const key = ["thread", t.id];
  const providers = useQuery({ queryKey: ["node-providers", t.node_id], queryFn: () => api.nodeProviders(t.node_id), staleTime: 60_000 });
  const [text, setText] = useState("");
  const dictation = useLocalDictation(text, setText, area, { panelShortcut: true, onEnter: (draft) => latestSendDraft.current(draft) });
  const showingDictation = !!dictation.preview;
  const dictating = useDictationInProgress();
  const picker = useRef<HTMLInputElement>(null);
  const [files, setFiles] = useState<Draft[]>([]);
  const quoteList = useRef<HTMLUListElement>(null);
  const codeTip = useCodeTip();
  const latestQuote = quotes.at(-1)?.id;
  useLayoutEffect(() => {
    const list = quoteList.current;
    if (list) list.scrollTop = list.scrollHeight;
  }, [latestQuote]);
  const uploading = files.some((f) => !f.done && !f.error);
  useLayoutEffect(() => { const el = area.current; if (el && !showingDictation) { el.style.height = "auto"; el.style.height = `${Math.min(el.scrollHeight, 240)}px`; } }, [dictation.value, showingDictation]);
  // Picked /commands, $skills and @files are tokens in the draft text.
  // The token reference preserves the full path of a selected file.
  const [tokens, setTokens] = useState<Tokens>({});
  const addToken = (insert: string, tk: Token) => {
    // Two files with the same name: the second one is written with its path.
    const k = tk.kind === "path" && tokens[insert] && tokens[insert].value !== tk.value ? `@${tk.value}` : insert;
    setTokens((ts) => ({ ...ts, [k]: tk }));
    return k;
  };
  const ranges = tokenRanges(text, tokens);
  // The harness receives full file paths and each quote with its own reply.
  const payload = (draft: string) => {
    const draftRanges = draft === text ? ranges : tokenRanges(draft, tokens);
    const outgoingText = draftRanges.reduceRight((value, range) => {
      const token = tokens[range.text];
      return token.kind === "path" ? `${value.slice(0, range.start)}@${token.value}${value.slice(range.end)}` : value;
    }, draft).trim();
    return [
      ...quotes.map((quote) => {
        if (quote.code) return codeRefText({ ...quote.code, code: quote.text, comment: quote.reply });
        const block = quoteBlock(quoteLabel(quote.text).replaceAll("»", "›"), quote.text).trim();
        return quote.reply ? `${block}\n\n**Respuesta a la cita**\n\n${quote.reply}` : block;
      }),
      outgoingText,
    ].filter(Boolean).join("\n\n");
  };
  const outgoing = payload(text);
  const paint = useRef<HTMLDivElement>(null);
  // A selection never splits a token: an edge inside one grows to cover it,
  // and a caret inside one jumps past it in the direction it was moving.
  const lastCaret = useRef(0);
  const snapSelection = () => {
    const el = area.current;
    if (!el) return;
    const { selectionStart: s0, selectionEnd: e0, selectionDirection: dir } = el;
    let a = s0, b = e0;
    for (const r of ranges) {
      if (s0 === e0) {
        if (s0 > r.start && s0 < r.end) {
          const prev = lastCaret.current;
          a = b = prev <= r.start ? r.end : prev >= r.end ? r.start : s0 - r.start < r.end - s0 ? r.start : r.end;
        }
      } else {
        if (a > r.start && a < r.end) a = r.start;
        if (b > r.start && b < r.end) b = r.end;
      }
    }
    if (a !== s0 || b !== e0) el.setSelectionRange(a, b, dir ?? undefined);
    lastCaret.current = a === b ? a : dir === "backward" ? a : b;
  };
  // A token deletes as a whole: Backspace at its end or inside, Delete at its start or inside.
  const deleteToken = (e: React.KeyboardEvent<HTMLTextAreaElement>) => {
    const el = e.currentTarget;
    if ((e.key !== "Backspace" && e.key !== "Delete") || el.selectionStart !== el.selectionEnd) return false;
    const at = el.selectionStart;
    const word = e.ctrlKey || e.altKey || e.metaKey;
    const reach = (r: { start: number; end: number }) => e.key === "Backspace"
      ? (at > r.start && at <= r.end) || (word && at > r.end && !text.slice(r.end, at).trim())
      : (at >= r.start && at < r.end) || (word && at < r.start && !text.slice(at, r.start).trim());
    const hit = ranges.find(reach);
    if (!hit) return false;
    e.preventDefault();
    const from = e.key === "Backspace" ? hit.start : Math.min(at, hit.start);
    const to = e.key === "Backspace" ? Math.max(at, hit.end) : hit.end;
    setText(text.slice(0, from) + text.slice(to));
    requestAnimationFrame(() => el.setSelectionRange(from, from));
    return true;
  };
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
  const availableProviders = (providers.data ?? []).filter(usable);
  const currentProvider = availableProviders.find((p) => p.id === t.provider);
  const currentModel = findModel(currentProvider?.models ?? [], t.model);
  const selectedModel = currentModel ? concreteModelId(currentModel) : t.model;
  const modelOptions = availableProviders.flatMap((provider) => explicitModels(provider.models).map((model) => ({
    value: JSON.stringify([provider.id, concreteModelId(model)]),
    modelId: concreteModelId(model),
    label: model.name,
    provider: provider.id,
    icon: <ProviderIcon driver={provider.id} model={model.slug} />,
    model,
  })));
  const pickerProviders: ModelProvider[] = availableProviders.map((p) => ({
    id: p.id, name: p.name, disabled: t.running && p.id !== t.provider,
  }));
  const send = useMutation({
    mutationFn: ({ message, fileIds }: { message: string; fileIds: string[]; quoteIds: string[] }) => api.send(t.id, message, fileIds),
    onSuccess: (_, { quoteIds }) => {
      setText("");
      setTokens({});
      onRemoveQuotes(quoteIds);
      files.forEach((f) => URL.revokeObjectURL(f.preview));
      setFiles([]);
      onSent();
      qc.invalidateQueries({ queryKey: key });
    },
  });
  const planning = useMutation({
    mutationFn: () => api.send(t.id, autoMessage("plan", { thread: t.id },
      "Planifica lo conversado sin implementar código. Usa ensure_thread_task para reutilizar la tarea vinculada o crearla y vincularla; después lee el plan con plan_read y crea o modifica el documento con plan_write o plan_edit. Haz las preguntas necesarias y usa el contexto de este hilo y su entorno. Solo el usuario acepta el plan.",
      "Planifica lo que hemos hablado en este hilo.")),
    onSuccess: () => { onSent(); return qc.invalidateQueries({ queryKey: key }); },
  });
  const stop = useMutation({ mutationFn: () => api.interrupt(t.id) });
  // A question the agent waits for sits on top of the box; the box writes the
  // free answer of the current question instead of a new message. A harness
  // question resolves its tool call; an ask_user one is answered by message.
  const native = t.requests?.find((r) => r.questions.length > 0);
  const ask = native ? undefined : openAsk(t.messages);
  const question = native ?? ask;
  const reply = useMutation({
    mutationFn: ({ answers, files }: { answers: string[][]; files: string[][] }) => native
      ? api.answer(t.id, native.id, answers, files)
      : api.send(t.id, askAnswerText(ask!.questions, answers), files.flat()),
    onSuccess: () => { onSent(); return qc.invalidateQueries({ queryKey: key }); },
  });
  const flow = useQuestionFlow(question?.id ?? "", question?.questions ?? [], (answers, files) => reply.mutate({ answers, files }));
  const asking = !!question && !!flow.q;
  const planQueued = t.queue.some((item) => item.text.includes(`<dory-context kind="plan" thread="${t.id}">`));
  const canPlan = t.local && !asking && !dictating && !planning.isPending && !send.isPending && !planQueued;
  useCommand("plan", () => { if (canPlan) planning.mutate(); }, canPlan);
  const settings = useMutation({
    scope: { id: `thread-settings:${t.id}` },
    mutationFn: (body: Parameters<typeof api.threadSettings>[1]) => api.threadSettings(t.id, body),
    onSuccess: (updated, change) => {
      if (change.model) rememberModel(updated);
      qc.setQueryData<ThreadView>(key, (current) => current && {
        ...current, provider: updated.provider, model: updated.model, effort: updated.effort, runtime_mode: updated.runtime_mode,
        reported_settings: current.provider === updated.provider && current.model === updated.model && current.effort === updated.effort
          ? current.reported_settings : undefined,
      });
      return qc.invalidateQueries({ queryKey: key });
    },
  });
  const suggest = useSuggest(t.id, t.provider, text, area, setText, addToken);
  const readyToSend = !uploading && !send.isPending && !settings.isPending;
  // While this box is dictating, sending finishes the dictation first and then sends it.
  const canSend = (!!outgoing || files.some((file) => file.done) || dictation.active) && readyToSend && (!dictating || dictation.active) && dictation.phase !== "finishing";
  // answerDraft gives what the box holds (typed, dictated, files) as the free
  // answer of the current question.
  const answerDraft = (draft: string) => {
    const fileIds = files.flatMap((file) => file.done ? [file.done.id] : []);
    const answer = draft.trim();
    if (reply.isPending || uploading || (!answer && !fileIds.length)) return;
    flow.answer(answer || "(ver adjuntos)", fileIds);
    setText("");
    setTokens({});
    files.forEach((f) => URL.revokeObjectURL(f.preview));
    setFiles([]);
  };
  const sendDraft = (draft: string) => {
    if (asking) { answerDraft(draft); return; }
    const message = payload(draft);
    const fileIds = files.flatMap((file) => file.done ? [file.done.id] : []);
    if (readyToSend && (message || fileIds.length)) send.mutate({ message, fileIds, quoteIds: quotes.map(({ id }) => id) });
  };
  const latestSendDraft = useRef(sendDraft);
  useLayoutEffect(() => { latestSendDraft.current = sendDraft; });
  const submit = () => {
    if (dictation.active) { dictation.finishAndSubmit((draft) => latestSendDraft.current(draft)); return; }
    if (asking) { answerDraft(text); return; }
    if (canSend) sendDraft(text);
  };
  if (!t.local) return null;
  const busy = t.running;
  return (
    <div className="mx-auto w-full max-w-[55.2rem] px-3 pb-4 pt-2 sm:px-6">
      <form data-quote-ui data-dictation-panel data-unsaved-draft={!!text.trim() || files.length > 0 || quotes.length > 0 || dictation.active} onSubmit={(e) => { e.preventDefault(); submit(); }}
        onDragOver={(e) => { if (e.dataTransfer.types.includes("Files")) e.preventDefault(); }}
        onDrop={(e) => { if (e.dataTransfer.files.length) { e.preventDefault(); addFiles(e.dataTransfer.files); } }}
        onPaste={(e) => {
          // Screenshots often come only as clipboard items, not in .files.
          const fs = Array.from(e.clipboardData.items).flatMap((i) => { const f = i.kind === "file" ? i.getAsFile() : null; return f ? [f] : []; });
          if (fs.length) { e.preventDefault(); addFiles(fs); }
        }}
        className={composerFrame}>
        {asking && <QuestionPanel flow={flow} busy={reply.isPending} error={reply.error} />}
        {!dictation.active && suggest.menu}
        {codeTip.card}
        {quotes.length > 0 && (
          <ul ref={quoteList} aria-label="Citas del mensaje" className="mx-2 mt-2 max-h-44 space-y-1.5 overflow-y-auto overscroll-contain px-1 py-1">
            {quotes.map((quote) => (
              <li key={quote.id} title={quote.code ? undefined : quote.reply ? `${quote.text}\n\n${quote.reply}` : quote.text}
                onPointerEnter={quote.code ? (e) => codeTip.show(e.currentTarget, quote) : undefined} onPointerLeave={quote.code ? codeTip.hide : undefined}
                className="flex min-w-0 items-center gap-2 rounded-lg bg-slate-50 px-2 py-1.5 text-sm dark:bg-raised">
                {quote.code ? <>
                  <Code weight="thin" aria-hidden="true" className="size-4 shrink-0 text-slate-500 dark:text-muted" />
                  <span className="min-w-0 flex-1 truncate font-mono text-xs">{quote.code.path.split("/").pop()}<span className="text-slate-500 dark:text-muted">:{quote.code.start === quote.code.end ? quote.code.start : `${quote.code.start}-${quote.code.end}`}</span>{quote.reply && <span className="ml-2 font-sans text-sm">{quote.reply}</span>}</span>
                </> : <>
                  <Quotes weight="thin" aria-hidden="true" className="size-4 shrink-0 text-slate-500 dark:text-muted" />
                  <span className="min-w-0 flex-1 truncate">{quote.text}</span>
                  {quote.reply && <span className="shrink-0 text-xs text-slate-500 dark:text-muted">Con respuesta</span>}
                </>}
                <button type="button" onClick={() => onRemoveQuotes([quote.id])} aria-label={quote.code ? `Quitar referencia ${quote.code.path}` : `Quitar cita ${quoteLabel(quote.text)}`}
                  className="grid size-6 shrink-0 place-items-center text-slate-500 hover:text-slate-900 dark:text-muted dark:hover:text-white">
                  <X weight="thin" className="size-4" />
                </button>
              </li>
            ))}
          </ul>
        )}
        {files.length > 0 && (
          <div className="flex flex-wrap gap-2 px-3 pt-3">
            {files.map((f) => (
              <div key={f.key} title={f.error ?? f.name} className={`group relative size-14 shrink-0 overflow-hidden rounded-xl border bg-slate-50 dark:bg-raised ${f.error ? "border-red-400" : "border-slate-200 dark:border-border"}`}>
                <a href={f.preview} target="_blank" rel="noreferrer" aria-label={`Ver ${f.name}`} className="block size-14 cursor-zoom-in">
                  {f.mime.startsWith("image/") ? <img src={f.preview} alt="" className="size-14 shrink-0 object-cover" /> : <span className="grid size-14 shrink-0 place-items-center text-slate-400"><FileText className="size-6" /></span>}
                </a>
                {f.error && <span className="absolute inset-x-0 bottom-0 bg-red-500 text-center text-[0.5625rem] font-medium text-white">Error</span>}
                {!f.done && !f.error && <span className="absolute inset-0 grid place-items-center bg-white/60 dark:bg-black/40"><span className="size-4 animate-spin rounded-full border-2 border-current border-t-transparent" /></span>}
                <button type="button" onClick={() => removeFile(f.key)} aria-label={`Quitar ${f.name}`} className="absolute right-1 top-1 grid size-5 place-items-center rounded-full bg-slate-900/70 text-white transition-opacity hover:bg-slate-900 sm:opacity-0 sm:group-hover:opacity-100 sm:focus-visible:opacity-100"><X weight="regular" className="size-3" /></button>
              </div>
            ))}
          </div>
        )}
        <div className="relative">
          {dictation.preview && <DictationPreview value={dictation.value} range={dictation.preview} format={(part) => paintTokens(part, tokens)} />}
          {/* Tokens show as objects: this layer paints the text under a see-through textarea, glyph for glyph, so the caret stays aligned. */}
          {!showingDictation && ranges.length > 0 && <div ref={paint} aria-hidden="true" className="pointer-events-none absolute inset-0 overflow-hidden whitespace-pre-wrap break-words px-4 pb-1 pt-3.5 text-[0.9375rem] leading-6">
            {paintTokens(text, tokens)}{"\u200b"}
          </div>}
          <textarea ref={area} data-dictation-disabled={send.isPending || undefined} rows={1} value={dictation.value} readOnly={dictation.active} onChange={(e) => setText(e.target.value)} onSelect={() => { if (!dictation.active) { snapSelection(); suggest.track(); } }} onClick={() => { if (!dictation.active) suggest.track(); }} aria-label="Mensaje"
            onScroll={(e) => { if (paint.current) paint.current.scrollTop = e.currentTarget.scrollTop; }}
            placeholder={asking ? (flow.q?.options?.length ? "Escribe otra respuesta…" : "Escribe tu respuesta…") : t.status === "waiting" ? "Escribe tu respuesta…" : "Escribe al hilo…"}
            onKeyDown={(e) => {
              if (e.nativeEvent.isComposing) return;
              if (dictation.active) {
                if (e.key === "Enter" && !e.shiftKey) {
                  e.preventDefault();
                  if (!e.repeat) dictation.finishAndSubmit((draft) => latestSendDraft.current(draft));
                }
                return;
              }
              if (suggest.onKeyDown(e) || deleteToken(e)) return;
              if (e.key === "Enter" && !e.shiftKey && window.matchMedia("(min-width: 640px)").matches) {
                e.preventDefault();
                if (!e.repeat) submit();
              }
            }}
            className={showingDictation ? "sr-only" : `${composerArea} ${ranges.length ? "text-transparent caret-slate-900 dark:caret-white" : ""}`} />
        </div>
        <div className="flex items-center gap-0.5 px-2 pb-2">
          <div className="flex min-w-0 flex-1 flex-wrap items-center gap-0.5">
            <input ref={picker} type="file" multiple hidden onChange={(e) => { if (e.target.files) addFiles(e.target.files); e.target.value = ""; }} />
            <button type="button" onClick={() => picker.current?.click()} title="Adjuntar imágenes o ficheros" aria-label="Adjuntar"
              className={iconButton}>
              <Paperclip weight="regular" className="size-4" />
            </button>
            <ModelPicker models={modelOptions} providers={pickerProviders} selected={JSON.stringify([t.provider, selectedModel])} effort={t.effort}
              modelId={t.reported_settings?.model ?? selectedModel} reportedEffort={t.reported_settings?.effort}
              effortsOf={(value) => {
                const option = modelOptions.find((model) => model.value === value);
                const efforts = option?.model.efforts;
                return option && efforts?.length ? { values: efforts, initial: modelEffort(option.provider, option.model) } : undefined;
              }}
              onChange={(value, effort) => {
                const option = modelOptions.find((model) => model.value === value);
                if (!option) return;
                const choice = { provider: option.provider, model: option.modelId, effort };
                if (settings.isPending || choice.provider !== t.provider || choice.model !== t.model || effort !== t.effort) settings.mutate(choice);
                else rememberModel(choice);
              }} />
          </div>
          <button type="button" onClick={() => planning.mutate()} disabled={!canPlan} title={planQueued ? "Planificación en cola" : `Planificar esta conversación (${formatChord(getShortcut("plan"))})`} aria-label="Planificar esta conversación" className={iconButton}>
            <ClipboardText weight="regular" className="size-4" />
          </button>
          <DictationMic dictation={dictation} disabled={send.isPending} />
          {busy && !asking && !outgoing && !files.length && !dictation.active ? (
            <button type="button" onClick={() => stop.mutate()} disabled={stop.isPending} title="Parar el turno en curso" aria-label="Parar"
              className={iconButton}>
              <Stop weight="fill" className="size-3.5" />
            </button>
          ) : (
            <button type="submit" disabled={asking ? (!text.trim() && !files.some((file) => file.done) && !dictation.active) || reply.isPending || uploading || (dictating && !dictation.active) || dictation.phase === "finishing" : !canSend} title={asking ? "Responder (Enter)" : busy ? "Encolar (Enter)" : "Enviar (Enter)"} aria-label={asking ? "Responder" : "Enviar"}
              className={iconButton}>
              {send.isPending || reply.isPending ? <span className="size-4 animate-spin rounded-full border-2 border-current border-t-transparent" /> : busy && !asking ? <ListBullets weight="regular" className="block size-4" /> : <ArrowUp weight="regular" className="block size-4" />}
            </button>
          )}
        </div>
        {Boolean(send.error || planning.error || settings.error || stop.error || dictation.error) && <div className="px-4 pb-2"><ErrorText error={send.error ?? planning.error ?? settings.error ?? stop.error ?? dictation.error} /></div>}
      </form>
    </div>
  );
}

type Suggestion = { key: string; label: string; hint?: string; insert: string; icon: React.ReactNode; token: Token };
const titles = { "/": "Comandos", $: "Skills", "@": "Ficheros" } as const;

// useSuggest autocompletes the composer as in v1: /command (only at the
// start), $skill and @file of the thread's folder. A pick replaces the typed
// word with how the harness invokes it.
function useSuggest(threadId: string, provider: string, text: string, area: React.RefObject<HTMLTextAreaElement | null>, setText: (t: string) => void, onToken: (insert: string, t: Token) => string) {
  const [word, setWord] = useState<{ start: number; sigil: "/" | "$" | "@"; q: string } | null>(null);
  const [index, setIndex] = useState(0);
  const catalog = useQuery({ queryKey: ["thread-commands", threadId, provider], queryFn: () => api.threadCommands(threadId), enabled: word?.sigil === "/" || word?.sigil === "$", staleTime: 60_000 });
  const files = useQuery({ queryKey: ["thread-files", threadId, word?.q], queryFn: () => api.threadFiles(threadId, word!.q), enabled: word?.sigil === "@", staleTime: 30_000 });
  const q = word?.q.toLowerCase() ?? "";
  const hit = (e: { name: string; description: string }) => !q || e.name.toLowerCase().includes(q) || e.description.toLowerCase().includes(q);
  const entry = (e: { name: string; description: string; insert: string }, icon: React.ReactNode, kind: "command" | "skill"): Suggestion => ({ key: e.insert, label: e.insert, hint: e.description, insert: e.insert, icon, token: { kind, value: e.name, title: e.description } });
  const list: Suggestion[] = !word ? []
    : word.sigil === "@" ? (files.data ?? []).map((f) => ({ key: f, label: f.split("/").pop()!, hint: f, insert: `@${f.split("/").pop()}`, icon: <FileIcon className="size-4 shrink-0 text-slate-400" />, token: { kind: "path", value: f } }))
    : word.sigil === "/" ? [...(catalog.data?.commands ?? []).filter(hit).map((e) => entry(e, <TerminalWindow className="size-4 shrink-0 text-slate-400" />, "command")),
      ...(catalog.data?.skills ?? []).filter(hit).map((e) => entry(e, <Lightning className="size-4 shrink-0 text-slate-400" />, "skill"))]
    : (catalog.data?.skills ?? []).filter(hit).map((e) => entry(e, <Lightning className="size-4 shrink-0 text-slate-400" />, "skill"));
  const shown = list.slice(0, 50);
  const track = () => {
    const el = area.current;
    if (!el) return;
    const before = text.slice(0, el.selectionStart);
    const m = /(^|\s)([/$@])([^\s/$@]*)$/.exec(before);
    const start = m ? before.length - m[3].length - 1 : -1;
    // A command only makes sense at the start of the message.
    if (!m || (m[2] === "/" && start !== 0)) { setWord(null); return; }
    setWord((w) => (w && w.start === start && w.q === m[3] ? w : { start, sigil: m[2] as "/" | "$" | "@", q: m[3] }));
    setIndex(0);
  };
  useEffect(track, [text]); // eslint-disable-line react-hooks/exhaustive-deps
  const pick = (s: Suggestion) => {
    if (!word) return;
    const end = word.start + 1 + word.q.length;
    const written = onToken(s.insert, s.token);
    const next = `${text.slice(0, word.start)}${written} ${text.slice(end).replace(/^ /, "")}`;
    setText(next);
    setWord(null);
    requestAnimationFrame(() => { const el = area.current; if (el) { const at = word.start + written.length + 1; el.focus(); el.setSelectionRange(at, at); } });
  };
  const onKeyDown = (e: React.KeyboardEvent) => {
    if (!shown.length) return false;
    if (e.key === "ArrowDown" || e.key === "ArrowUp") { e.preventDefault(); setIndex((i) => (i + (e.key === "ArrowDown" ? 1 : shown.length - 1)) % shown.length); return true; }
    if (e.key === "Enter" || e.key === "Tab") { e.preventDefault(); pick(shown[index]); return true; }
    if (e.key === "Escape") { e.preventDefault(); setWord(null); return true; }
    return false;
  };
  const loading = word && (word.sigil === "@" ? files.isPending : catalog.isPending);
  const menu = word && (shown.length > 0 || loading || word.sigil !== "@") && (
    <div className="absolute bottom-full left-3 z-30 mb-2 w-[min(32rem,calc(100%-1.5rem))] overflow-hidden rounded-xl border border-slate-200 bg-white shadow-lg dark:border-border dark:bg-surface">
      <p className="px-3 pb-1 pt-2 text-[0.6875rem] font-semibold uppercase tracking-wide text-slate-400">{titles[word.sigil]}</p>
      <ul role="listbox" aria-label={titles[word.sigil]} className="max-h-64 overflow-y-auto p-1">
        {shown.map((s, i) => (
          <li key={s.key}>
            <button type="button" role="option" aria-selected={i === index} onMouseDown={(e) => { e.preventDefault(); pick(s); }} onMouseEnter={() => setIndex(i)}
              className={`flex w-full items-center gap-2 rounded-lg px-2 py-1.5 text-left text-sm ${i === index ? "bg-slate-100 dark:bg-raised" : ""}`}>
              {s.icon}
              <span className="shrink-0 font-medium">{s.label}</span>
              {s.hint && <span className="min-w-0 truncate text-xs text-slate-500 dark:text-muted">{s.hint}</span>}
            </button>
          </li>
        ))}
        {!shown.length && <li className="px-2 py-1.5 text-xs text-slate-400">{loading ? "Cargando…" : word.sigil === "/" ? "Este harness no tiene comandos." : "No hay skills que coincidan."}</li>}
      </ul>
    </div>
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
  const dictating = useDictationInProgress();
  const save = () => {
    if (dictating || draft === null) return;
    onEdit(draft);
    setDraft(null);
  };
  return (
    <div className="msg-in flex flex-col items-end gap-1">
      <div className="w-fit max-w-[85%] rounded-2xl rounded-br-md border border-dashed border-slate-400 px-4 py-2.5 text-slate-700 dark:border-border-strong dark:text-text-secondary">
        {draft === null ? <div className="flex flex-col text-sm"><MessageParts text={text} /></div> : (
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
function useCodeTip() {
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
