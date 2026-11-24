import { useEffect, useLayoutEffect, useRef, useState, useSyncExternalStore, type RefObject } from "react";
import { hasDictationPopupOutside, isTextInput, useDictationShortcut } from "./dictation-shortcut";
import { wav } from "./audio-wav";
import { playDictationSound, prepareDictationSound } from "./dictation-sound";
import { openDictationMicrophone } from "./dictation-microphone";
import { events } from "../../lib/store";
import { SpeechModel, modelsPath, useSpeechModels, useSpeechModel } from "./speech-models";

type Draft = { source: string; start: number; end: number };
type Session = {
  draft: Draft; model: SpeechModel; abort: AbortController; recording: Float32Array[];
  releaseMic: () => void; phase: Exclude<Phase, "idle">;
  completion: Promise<string | undefined>; complete: (text?: string) => void; submitQueued?: boolean; started: Promise<void>; markStarted: () => void;
  transcript: string; partial: string; socket?: WebSocket; stopCapture?: () => void; timeout?: number;
  onCaptureFrame?: () => void;
};
export type Phase = "idle" | "starting" | "recording" | "finishing";

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

export const meterBars = 5;
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
  options?: { panelShortcut?: boolean; onEnter?: (steer: boolean) => void }) {
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
      if (!e.repeat) onEnter.current?.(e.ctrlKey || e.metaKey);
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
