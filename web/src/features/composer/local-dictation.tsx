import { useEffect, useLayoutEffect, useRef, useState, useSyncExternalStore, type ReactNode, type RefObject } from "react";
import { useQuery } from "@tanstack/react-query";
import { CircleNotch, Microphone } from "@phosphor-icons/react";
import { req } from "../../api/client";
import { formatChord, hasDictationPopupOutside, isTextInput, useDictationShortcut, useDictationShortcutSetting } from "./dictation-shortcut";
import { wav } from "./audio-wav";
import { playDictationSound, prepareDictationSound } from "./dictation-sound";
import { openDictationMicrophone } from "./dictation-microphone";
import { iconButton } from "../../components/ui";
import { modelKey, useInputSettingsSync } from "../settings/input-settings";
import { qk } from "../../api/queries/keys";
import { events } from "../../lib/store";
import { storage } from "../../lib/storage";

export type SpeechModel = {
  id: string; name: string; bytes: number; streaming: boolean;
  status: "available" | "downloading" | "installed" | "failed"; error?: string;
};
export type SpeechCatalog = { runtime: boolean; models: SpeechModel[] };

const modelsPath = "/dictation/models";
export const speechKey = qk.speechModels();
const preferenceKey = modelKey;

export function useSpeechModels() {
  return useQuery({
    queryKey: speechKey, queryFn: () => req<SpeechCatalog>("GET", modelsPath),
    refetchInterval: (query) => query.state.data?.models.some((model) => model.status === "downloading") ? 1000 : false,
  });
}

function subscribe(notify: () => void) {
  const off = events.dictationModelChanged.on(notify);
  window.addEventListener("storage", notify);
  return () => { off(); window.removeEventListener("storage", notify); };
}

// pickSpeechModel is the model a node uses: the chosen one if it is installed,
// otherwise the best installed one.
export function pickSpeechModel(catalog: SpeechCatalog | undefined, id: string | null | undefined) {
  if (!catalog?.runtime) return undefined;
  const models = catalog.models.filter((model) => model.status === "installed");
  return models.find((model) => model.id === id) ?? models.find((model) => model.streaming) ?? models[0];
}

// useSpeechModel is the model of this node, kept in sync with its settings.
export function useSpeechModel(catalog?: SpeechCatalog) {
  useInputSettingsSync();
  const id = useSyncExternalStore(subscribe, () => {
    try { return storage.get(preferenceKey); } catch { return null; }
  });
  return pickSpeechModel(catalog, id);
}

export const downloadSpeechModel = (id: string) => req<SpeechCatalog>("POST", `${modelsPath}/${encodeURIComponent(id)}/download`);

type Draft = { source: string; start: number; end: number };
type Session = {
  draft: Draft; model: SpeechModel; abort: AbortController; recording: Float32Array[];
  releaseMic: () => void; phase: Exclude<Phase, "idle">;
  completion: Promise<string | undefined>; complete: (text?: string) => void; submitQueued?: boolean; started: Promise<void>; markStarted: () => void;
  transcript: string; partial: string; socket?: WebSocket; stopCapture?: () => void; timeout?: number;
  onCaptureFrame?: () => void;
};
type Phase = "idle" | "starting" | "recording" | "finishing";

let microphoneOwner: symbol | undefined;
const isDictating = () => microphoneOwner !== undefined;
const subscribeActivity = (notify: () => void) => {
  return events.dictationActivityChanged.on(notify);
};

export function useDictationInProgress() {
  return useSyncExternalStore(subscribeActivity, isDictating);
}

export function reserveDictation() {
  if (microphoneOwner) return undefined;
  const owner = Symbol();
  microphoneOwner = owner;
  events.dictationActivityChanged.emit();
  return () => {
    if (microphoneOwner !== owner) return;
    microphoneOwner = undefined;
    events.dictationActivityChanged.emit();
  };
}

export function insertDictation(draft: Draft, spoken: string) {
  const text = spoken.trim();
  if (!text) return { text: draft.source, cursor: draft.start };
  const before = draft.source.slice(0, draft.start);
  const after = draft.source.slice(draft.end);
  const left = before && !/\s$/.test(before) ? " " : "";
  const right = after && !/^[\s.,!?;:)}\]]/.test(after) ? " " : "";
  const inserted = before + left + text;
  return { text: inserted + right + after, cursor: inserted.length };
}

const transcriptOf = (session: Session) => [session.transcript, session.partial].filter(Boolean).join(" ");
const path = (model: SpeechModel, action: string) => `/api${modelsPath}/${encodeURIComponent(model.id)}/${action}`;

function stopCapture(session: Session) {
  const stop = session.stopCapture;
  session.stopCapture = undefined;
  stop?.();
}

function release(session: Session) {
  window.clearTimeout(session.timeout);
  stopCapture(session);
  session.abort.abort();
  session.socket?.close();
  session.releaseMic();
}

// Downsample to the 16 kHz WAV format shared with the existing dictation API.
function resample(input: Float32Array, sampleRate: number) {
  const ratio = sampleRate / 16000;
  return Float32Array.from({ length: Math.floor(input.length / ratio) }, (_, index) => input[Math.floor(index * ratio)]);
}

function pcm16(samples: Float32Array) {
  const bytes = new DataView(new ArrayBuffer(samples.length * 2));
  samples.forEach((sample, index) => bytes.setInt16(index * 2, Math.round(Math.max(-1, Math.min(1, sample)) * 32767), true));
  return bytes.buffer;
}

const meterBars = 5;
const meterInterval = 0.08;

function voiceLevel(samples: Float32Array) {
  let energy = 0;
  for (const sample of samples) energy += sample * sample;
  const decibels = 20 * Math.log10(Math.sqrt(energy / samples.length));
  return Math.max(0, Math.min(1, (decibels + 48) / 30));
}

// Both recorders feed their captured audio to the same button animation.
export function createDictationMeter(meter: RefObject<HTMLSpanElement | null>) {
  const levels = Array<number>(meterBars).fill(0);
  let lastFrame = -Infinity;
  return (samples: Float32Array, time: number) => {
    const bars = meter.current?.children;
    if (!bars || time - lastFrame < meterInterval) return;
    lastFrame = time;
    levels.push(voiceLevel(samples));
    levels.shift();
    levels.forEach((level, index) => {
      const bar = bars.item(index);
      if (bar instanceof HTMLElement) bar.style.setProperty("--meter-level", String(0.16 + 0.84 * level));
    });
  };
}

export function useLocalDictation(text: string, setText: (text: string) => void, area: RefObject<HTMLTextAreaElement | null>,
  options?: { panelShortcut?: boolean; onEnter?: (draft: string) => void }) {
  const catalog = useSpeechModels();
  const model = useSpeechModel(catalog.data);
  const [phase, setPhase] = useState<Phase>("idle");
  const [preview, setPreview] = useState("");
  const [error, setError] = useState<unknown>(null);
  const current = useRef<Session | undefined>(undefined);
  const caret = useRef<number | undefined>(undefined);
  const meter = useRef<HTMLSpanElement>(null);

  useEffect(() => () => {
    const session = current.current;
    current.current = undefined;
    if (session) { release(session); session.complete(); }
  }, []);

  useLayoutEffect(() => {
    if (caret.current === undefined || phase !== "idle") return;
    area.current?.focus({ preventScroll: true });
    area.current?.setSelectionRange(caret.current, caret.current);
    caret.current = undefined;
  }, [area, phase, text]);

  const end = (session: Session, result = transcriptOf(session), failure?: unknown, cancelled = false) => {
    if (current.current !== session) return;
    current.current = undefined;
    release(session);
    const insertion = result.trim() ? insertDictation(session.draft, result) : undefined;
    if (insertion) {
      caret.current = insertion.cursor;
      setText(insertion.text);
    }
    setPreview("");
    setError(failure ?? null);
    setPhase("idle");
    // With nothing new transcribed, the text already written is still sent.
    session.complete(failure || cancelled ? undefined : insertion?.text ?? session.draft.source);
  };

  const finish = (session: Session) => {
    if (session.phase !== "recording") return session.completion;
    session.phase = "finishing";
    setPhase("finishing");
    window.clearTimeout(session.timeout);
    // Drain the queued frame and the frame still being filled at the stop request.
    let framesRemaining = 2;
    const completeCapture = () => {
      if (current.current !== session) return;
      session.onCaptureFrame = undefined;
      commitCapture(session);
    };
    session.onCaptureFrame = () => {
      if (--framesRemaining === 0) completeCapture();
    };
    // A suspended audio context may stop delivering frames.
    session.timeout = window.setTimeout(completeCapture, 500);
    return session.completion;
  };

  const commitCapture = (session: Session) => {
    stopCapture(session);
    playDictationSound("stop");
    window.clearTimeout(session.timeout);
    session.timeout = window.setTimeout(() => end(session, undefined, new Error("El modelo tardó demasiado en terminar el dictado.")), 60000);
    if (session.socket) {
      try {
        session.socket.send(JSON.stringify({ type: "input_audio_buffer.commit" }));
      } catch (failure) {
        end(session, undefined, failure);
      }
    } else {
      void finalizeBatch(session);
    }
  };

  const finalizeBatch = async (session: Session) => {
    try {
      const audio = new Float32Array(session.recording.reduce((size, frame) => size + frame.length, 0));
      if (!audio.length) { end(session, ""); return; }
      let offset = 0;
      for (const frame of session.recording) { audio.set(frame, offset); offset += frame.length; }
      session.recording.length = 0;
      const body = new FormData();
      body.set("file", wav(audio), "dictation.wav");
      body.set("response_format", "json");
      const response = await fetch(path(session.model, "transcribe"), { method: "POST", body, signal: session.abort.signal });
      const data = await response.json().catch(() => ({}));
      if (!response.ok) throw new Error(data.error?.message ?? "No se pudo transcribir el audio.");
      end(session, data.text ?? "");
    } catch (failure) {
      end(session, undefined, failure);
    }
  };

  const finishAndSubmit = (submit: (text: string) => void) => {
    const session = current.current;
    if (!session || session.submitQueued) return;
    session.submitQueued = true;
    // Enter while the microphone is still opening finishes as soon as it records.
    void (session.phase === "starting" ? session.started.then(() => finish(session)) : finish(session)).then((final) => { if (final !== undefined) submit(final); });
  };

  const start = async () => {
    if (!model || current.current || area.current?.hasAttribute("data-dictation-disabled")) return;
    if (!window.isSecureContext || !navigator.mediaDevices) {
      setError(new Error("El micrófono requiere HTTPS o localhost."));
      return;
    }
    const releaseMic = reserveDictation();
    if (!releaseMic) return;
    prepareDictationSound();
    const startAt = area.current?.selectionStart ?? text.length;
    let complete: Session["complete"] = () => {};
    const completion = new Promise<string | undefined>((resolve) => { complete = resolve; });
    let markStarted = () => {};
    const started = new Promise<void>((resolve) => { markStarted = resolve; });
    const session: Session = {
      completion, complete, started, markStarted, phase: "starting",
      draft: { source: text, start: startAt, end: area.current?.selectionEnd ?? startAt },
      model, releaseMic, abort: new AbortController(), recording: [], transcript: "", partial: "",
    };
    current.current = session;
    setError(null);
    setPhase("starting");
    area.current?.focus({ preventScroll: true });
    try {
      const media = await openDictationMicrophone();
      if (session.abort.signal.aborted) { media.getTracks().forEach((track) => track.stop()); return; }
      const context = new AudioContext();
      session.stopCapture = () => {
        media.getTracks().forEach((track) => track.stop());
        if (context.state !== "closed") void context.close().catch(() => {});
      };
      await context.resume();
      if (session.abort.signal.aborted) return;
      const source = context.createMediaStreamSource(media);
      const processor = context.createScriptProcessor(4096, 1, 1);
      const stop = session.stopCapture;
      session.stopCapture = () => {
        processor.onaudioprocess = null;
        processor.disconnect();
        source.disconnect();
        stop();
      };

      if (model.streaming) {
        const address = new URL(path(model, "stream"), location.href);
        address.protocol = location.protocol === "https:" ? "wss:" : "ws:";
        const socket = new WebSocket(address);
        session.socket = socket;
        await new Promise<void>((resolve, reject) => {
          const timeout = window.setTimeout(() => reject(new Error("El modelo no abrió la sesión de dictado.")), 40000);
          session.abort.signal.addEventListener("abort", () => {
            window.clearTimeout(timeout);
            reject(new Error("Dictado cancelado."));
          }, { once: true });
          socket.onopen = () => socket.send(JSON.stringify({
            type: "session.update", session: { sample_rate: context.sampleRate, language: "es-ES" },
          }));
          socket.onmessage = ({ data }) => {
            if (current.current !== session) return;
            const event = JSON.parse(data);
            const type: string = event.type;
            if (type === "session.updated") {
              window.clearTimeout(timeout);
              resolve();
            } else if (type.endsWith(".delta")) {
              session.partial += event.delta ?? "";
              setPreview(transcriptOf(session));
            } else if (type.endsWith(".completed")) {
              session.transcript = [session.transcript, event.transcript].filter(Boolean).join(" ");
              session.partial = "";
              setPreview(session.transcript);
            } else if (type === "input_audio_buffer.committed") {
              end(session);
            } else if (type === "error") {
              end(session, undefined, new Error(event.error?.message ?? "Falló el dictado en directo."));
            }
          };
          socket.onclose = () => end(session, undefined, new Error("Se perdió la conexión con el modelo de dictado."));
          socket.onerror = () => end(session, undefined, new Error("No se pudo conectar con el modelo de dictado."));
        });
      }
      if (session.abort.signal.aborted) return;
      const updateMeter = createDictationMeter(meter);
      processor.onaudioprocess = (event) => {
        const samples = event.inputBuffer.getChannelData(0);
        updateMeter(samples, context.currentTime);
        if (session.socket) {
          if (session.socket.readyState !== WebSocket.OPEN) return;
          if (session.socket.bufferedAmount > 512 * 1024) {
            end(session, undefined, new Error("El modelo no puede seguir el ritmo del micrófono."));
            return;
          }
          session.socket.send(pcm16(samples));
        } else {
          session.recording.push(resample(samples, context.sampleRate));
        }
        session.onCaptureFrame?.();
      };
      source.connect(processor);
      processor.connect(context.destination);
      session.timeout = window.setTimeout(() => void finish(session), 5 * 60 * 1000);
      session.phase = "recording";
      setPhase("recording");
      playDictationSound("start");
      session.markStarted();
    } catch (failure) {
      end(session, undefined, failure);
    }
  };

  const toggle = () => {
    const session = current.current;
    if (!session) return start();
    if (session.phase === "starting") end(session, "", undefined, true);
    else if (session.phase === "recording") void finish(session);
  };
  useDictationShortcut((event, context) => {
    const input = area.current;
    if (!input) return false;
    if (context === "active") {
      if (!current.current) return false;
    } else {
      if (!model || input.disabled || input.hasAttribute("data-dictation-disabled") || isDictating() || !(event.target instanceof Element)) return false;
      const target = event.target;
      const panel = input.closest("[data-dictation-panel]");
      if (context === "editor") {
        if (hasDictationPopupOutside(input) || (target !== input && (!panel || target.closest("[data-dictation-panel]") !== panel || isTextInput(target)))) return false;
      } else if (context === "panel") {
        if (!options?.panelShortcut || isTextInput(target) || hasDictationPopupOutside()) return false;
      } else {
        return false;
      }
    }
    void toggle();
    return true;
  });

  // Enter keeps sending while dictating even after a click or a text selection
  // took the focus away from the box.
  const onEnter = useRef(options?.onEnter);
  onEnter.current = options?.onEnter;
  useEffect(() => {
    if (phase === "idle" || !onEnter.current) return;
    const handle = (e: KeyboardEvent) => {
      if (e.key !== "Enter" || e.shiftKey || e.isComposing || e.defaultPrevented) return;
      const target = e.target instanceof Element ? e.target : null;
      if (target && (target === area.current || isTextInput(target) || hasDictationPopupOutside())) return;
      e.preventDefault();
      if (!e.repeat) finishAndSubmit((draft) => onEnter.current?.(draft));
    };
    window.addEventListener("keydown", handle, true);
    return () => window.removeEventListener("keydown", handle, true);
  });

  const draft = current.current?.draft;
  const provisional = draft && preview.trim() ? insertDictation(draft, preview) : undefined;
  const provisionalRange = provisional ? { start: provisional.cursor - preview.trim().length, end: provisional.cursor } : undefined;
  return {
    active: phase !== "idle", phase, model, error, meter,
    value: provisional?.text ?? text, preview: provisionalRange,
    toggle, start, finishAndSubmit,
  };
}

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

type DictationControl = {
  phase: Phase;
  active: boolean;
  model?: { name: string };
  meter: RefObject<HTMLSpanElement | null>;
  toggle: () => void;
};

function DictationTimer() {
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

export function DictationMic({ dictation, disabled }: {
  dictation: DictationControl;
  disabled?: boolean;
}) {
  const { phase, model } = dictation;
  const shortcut = formatChord(useDictationShortcutSetting());
  const activeElsewhere = useDictationInProgress() && !dictation.active;
  const recording = phase === "recording";
  const busy = phase === "starting" || phase === "finishing";
  const label = recording ? "Parar dictado" : phase === "starting" ? "Preparando dictado" : phase === "finishing" ? "Transcribiendo" : "Dictar";
  const unavailable = disabled || busy || activeElsewhere || !model;
  const hint = activeElsewhere ? "Termina el otro dictado para usar este micrófono" : !model ? "Configura un modelo de dictado en Ajustes" : phase === "idle" ? `Dictar con ${model.name}` : label;
  return (
    <span title={`${hint} (${shortcut})`} className="inline-flex">
    <button type="button" aria-label={label} aria-pressed={recording} disabled={unavailable} onClick={dictation.toggle}
      className={`${recording ? `${iconButton} gap-1.5 px-2` : iconButton} disabled:pointer-events-none`}>
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
