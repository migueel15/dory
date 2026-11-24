import { ArrowUp,ClipboardText,Code,ListBullets,Quotes,Stop,X } from "@phosphor-icons/react";
import { useQueryClient } from "@tanstack/react-query";
import { useLayoutEffect,useRef,useState } from "react";
import { api,usable,type ThreadView } from "../../api/client";
import { useNodeProviders } from "../../api/queries/hooks";
import { qk } from "../../api/queries/keys";
import { useAnswerQuestion,useAnswerSecret,useInterruptThread,useSendMessage,useSendText,useThreadSettings } from "../../api/queries/threads";
import { useCommand,useShortcuts } from "../../components/shortcuts";
import { ErrorText,iconButton } from "../../components/ui";
import { modelId as concreteModelId,explicitModels,findModel,modelEffort,rememberModel } from "../../effort";
import { autoMessage } from "../../lib/auto-message";
import { events,useEmitter } from "../../lib/store";
import { codeRefText } from "../chat/code-ref";
import { QuestionPanel,askAnswerText,openAsk,useQuestionFlow } from "../chat/question-card";
import type { DraftQuote } from "../chat/quote-selection";
import { SecretPanel } from "../chat/secret-card";
import { useComposerFiles } from "./composer-files";
import { paintTokens,quoteBlock,quoteLabel,tokenRanges,type Token,type Tokens } from "./composer-tokens";
import { formatChord } from "./dictation-shortcut";
import { DictationMic,DictationPreview } from "./dictation-ui";
import { ProviderIcon } from "./launch-picker";
import { useDictationInProgress,useLocalDictation } from "./local-dictation";
import { ModelPicker,ModelProvider } from "./model-picker";
import { useCodeTip } from "./queue";
import { threadSource,useSuggest } from "./suggest";

// The composer of a thread, kept from Dory v1: attachments, @ mentions of
// files, model, effort and permissions, and send, queue or stop.

export const composerFrame = "relative z-10 rounded-3xl border border-slate-200 bg-white shadow-md dark:border-raised dark:bg-surface";
export const composerArea = "relative block max-h-60 w-full resize-none break-words bg-transparent px-4 pb-1 pt-3.5 text-[0.9375rem] leading-6 !outline-none placeholder:text-slate-400";

export function Composer({ t, onSent, area, quotes, onRemoveQuotes }: {
  t: ThreadView;
  onSent: () => void;
  area: React.RefObject<HTMLTextAreaElement | null>;
  quotes: DraftQuote[];
  onRemoveQuotes: (ids: string[]) => void;
}) {
  const qc = useQueryClient();
  const getShortcut = useShortcuts();
  const key = qk.thread(t.id);
  const providers = useNodeProviders(t.node_id, { staleTime: 60_000 });
  const [text, setText] = useState("");
  useEmitter(events.fillComposer, (d) => { if (d.threadId === t.id) { setText(d.text); area.current?.focus(); } });
  const dictation = useLocalDictation(text, setText, area, { panelShortcut: true, onEnter: (steer) => latestSubmit.current(steer) });
  const showingDictation = !!dictation.preview;
  const dictating = useDictationInProgress();
  // Each file uploads as soon as it is added; the message carries the results.
  const attach = useComposerFiles((file) => api.upload(t.id, file));
  const files = attach.files;
  const quoteList = useRef<HTMLUListElement>(null);
  const codeTip = useCodeTip();
  const latestQuote = quotes.at(-1)?.id;
  useLayoutEffect(() => {
    const list = quoteList.current;
    if (list) list.scrollTop = list.scrollHeight;
  }, [latestQuote]);
  const uploading = attach.uploading;
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
  const send = useSendMessage<{ message: string; fileIds: string[]; quoteIds: string[]; steer?: boolean }>(t.id, {
    onSuccess: (_, { quoteIds }) => {
      setText("");
      setTokens({});
      onRemoveQuotes(quoteIds);
      attach.clear();
      onSent();
      qc.invalidateQueries({ queryKey: key });
    },
  });
  const planning = useSendText(t.id, autoMessage("plan", { thread: t.id },
    "Planifica lo conversado sin implementar código. Usa ensure_thread_task para reutilizar la tarea vinculada o crearla y vincularla; después lee el plan con plan_read y crea o modifica el documento con plan_write o plan_edit. Haz las preguntas necesarias y usa el contexto de este hilo y su entorno. Solo el usuario acepta el plan.",
    "Planifica lo que hemos hablado en este hilo."), {
    onSuccess: () => { onSent(); return qc.invalidateQueries({ queryKey: key }); },
  });
  const stop = useInterruptThread(t.id);
  // A question the agent waits for sits on top of the box; the box writes the
  // free answer of the current question instead of a new message. A harness
  // question resolves its tool call; an ask_user one is answered by message.
  const native = t.requests?.find((r) => r.questions?.length);
  const ask = native ? undefined : openAsk(t.messages);
  const question = native ?? ask;
  const reply = useAnswerQuestion(t.id, native?.id, (answers) => askAnswerText(ask!.questions, answers), {
    onSuccess: () => { onSent(); return qc.invalidateQueries({ queryKey: key }); },
  });
  const flow = useQuestionFlow(question?.id ?? "", question?.questions ?? [], (answers, files) => reply.mutate({ answers, files }));
  const secret = t.requests?.find((r) => r.kind === "secret" && r.secret);
  const answerSecret = useAnswerSecret(t.id, secret?.id, {
    onSuccess: () => qc.invalidateQueries({ queryKey: key }),
  });
  const asking = !!question && !!flow.q;
  const planQueued = t.queue.some((item) => item.text.includes(`<dory-context kind="plan" thread="${t.id}">`));
  const canPlan = t.local && !asking && !dictating && !planning.isPending && !send.isPending && !planQueued;
  useCommand("plan", () => { if (canPlan) planning.mutate(); }, canPlan);
  const settings = useThreadSettings(t.id, {
    scope: { id: `thread-settings:${t.id}` },
    onSuccess: (updated, change) => {
      if (change.model) rememberModel(updated);
      qc.setQueryData<ThreadView>(key, (current) => current && {
        ...current, provider: updated.provider, model: updated.model, effort: updated.effort, runtime_mode: updated.runtime_mode,
        reported_settings: current.provider === updated.provider && current.model === updated.model && current.effort === updated.effort
          ? current.reported_settings : {},
      });
      return qc.invalidateQueries({ queryKey: key });
    },
  });
  const suggest = useSuggest(threadSource(t.id, t.provider), text, area, setText, addToken);
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
    attach.clear();
  };
  const sendDraft = (draft: string, steer = false) => {
    if (asking) { answerDraft(draft); return; }
    const message = payload(draft);
    const fileIds = files.flatMap((file) => file.done ? [file.done.id] : []);
    if (readyToSend && (message || fileIds.length)) send.mutate({ message, fileIds, quoteIds: quotes.map(({ id }) => id), steer });
  };
  const latestSendDraft = useRef(sendDraft);
  useLayoutEffect(() => { latestSendDraft.current = sendDraft; });
  // submit is the one way out of the box: dictating, it finishes the dictation
  // first and then sends what it left, queued or steering alike.
  const submit = (steer = false) => {
    if (dictation.active) { dictation.finishAndSubmit((draft) => latestSendDraft.current(draft, steer)); return; }
    if (asking) { answerDraft(text); return; }
    if (canSend) sendDraft(text, steer);
  };
  const latestSubmit = useRef(submit);
  useLayoutEffect(() => { latestSubmit.current = submit; });
  if (!t.local) return null;
  const busy = t.running;
  return (
    <div className="mx-auto w-full max-w-[55.2rem] px-3 pb-4 pt-2 sm:px-6">
      <form data-quote-ui data-dictation-panel data-unsaved-draft={!!text.trim() || files.length > 0 || quotes.length > 0 || dictation.active} onSubmit={(e) => { e.preventDefault(); submit(); }}
        {...attach.formProps}
        className={composerFrame}>
        {secret ? <SecretPanel key={secret.id} request={secret} busy={answerSecret.isPending} error={answerSecret.error}
          onSend={(values) => answerSecret.mutate({ values })} onDecline={() => answerSecret.mutate({ decline: true })} />
          : asking && <QuestionPanel flow={flow} busy={reply.isPending} error={reply.error} />}
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
                  <X weight="regular" className="size-4" />
                </button>
              </li>
            ))}
          </ul>
        )}
        {attach.tray}
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
                  if (!e.repeat) submit(e.ctrlKey || e.metaKey);
                }
                return;
              }
              if (suggest.onKeyDown(e) || deleteToken(e)) return;
              if (e.key === "Enter" && !e.shiftKey && window.matchMedia("(min-width: 640px)").matches) {
                e.preventDefault();
                if (!e.repeat) submit(e.ctrlKey || e.metaKey);
              }
            }}
            className={showingDictation ? "sr-only" : `${composerArea} ${ranges.length ? "text-transparent caret-slate-900 dark:caret-white" : ""}`} />
        </div>
        <div className="flex items-center gap-0.5 px-2 pb-2">
          <div className="flex min-w-0 flex-1 flex-wrap items-center gap-0.5">
            {attach.attachButton}
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
            <button type="submit" disabled={asking ? (!text.trim() && !files.some((file) => file.done) && !dictation.active) || reply.isPending || uploading || (dictating && !dictation.active) || dictation.phase === "finishing" : !canSend} title={asking ? "Responder (Enter)" : busy ? "Encolar (Enter) · Mandar ya (Ctrl+Enter)" : "Enviar (Enter)"} aria-label={asking ? "Responder" : "Enviar"}
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
