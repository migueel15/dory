import { useEffect, useRef, useState } from "react";
import { hasDictationPopupOutside, isTextInput, useDictationShortcut } from "./dictation-shortcut";
import { createDictationMeter, reserveDictation, useDictationInProgress } from "./local-dictation";
import { DictationMic } from "./dictation-ui";
import { playDictationSound, prepareDictationSound } from "./dictation-sound";
import { openDictationMicrophone } from "./dictation-microphone";
import { wav } from "./audio-wav";
import { apiError } from "../../api/client";
import { Button, ErrorText, Textarea } from "../../components/ui";

type DictationState = { id: string; status: string; text: string; error?: string };
export type DictationTarget = { kind: "capture"; project_id?: string } | { kind: "thread"; thread_id: string };

const RATE = 16000;
const FRAME = RATE / 10; // 100 ms frames to measure energy
const SILENCE = 0.01; // RMS below this counts as silence (same as the server)
const PAUSE = 10; // 1 s of silence after enough voice sends the batch
const MIN_VOICE = 15; // 1.5 s of voice before a pause sends it
const MIN_BATCH = 4 * RATE; // from 4 s, cut at the first silent frame
const MAX_BATCH = 6 * RATE; // at 6 s, cut at the quietest frame
const OVERLAP = RATE / 2; // 0.5 s repeated after a forced cut
const LONG_GAP = 6; // silences over 0.6 s ...
const KEEP_GAP = 3; // ... are shortened to 0.3 s

async function call<T>(method: string, path: string, body?: BodyInit, json = true): Promise<T> {
  const res = await fetch(`/api/dictation${path}`, { method, credentials: "same-origin", body, headers: body && json ? { "Content-Type": "application/json" } : body ? { "Content-Type": "audio/wav" } : undefined });
  if (!res.ok) {
    throw await apiError(res);
  }
  return res.json();
}

function rms(a: Float32Array, from: number) {
  let sum = 0;
  for (let i = from; i < from + FRAME; i++) sum += a[i] * a[i];
  return Math.sqrt(sum / FRAME);
}

function voiced(a: Float32Array) {
  for (let f = 0; f + FRAME <= a.length; f += FRAME) if (rms(a, f) >= SILENCE) return true;
  return false;
}

// cutPoint says where to end the batch and whether the next one repeats an
// overlap; cut 0 keeps recording. A pause after enough voice, then the first
// silent frame from MIN_BATCH, then the quietest frame at MAX_BATCH.
function cutPoint(a: Float32Array): { cut: number; overlap: boolean } {
  let voice = 0, run = 0, best = MIN_BATCH, bestRms = Infinity;
  for (let f = 0; f + FRAME <= Math.min(a.length, MAX_BATCH); f += FRAME) {
    const r = rms(a, f);
    if (r < SILENCE) run++; else { run = 0; voice++; }
    if (run >= PAUSE && voice >= MIN_VOICE) return { cut: f + FRAME, overlap: false };
    if (f >= MIN_BATCH) {
      if (r < SILENCE) return { cut: f + FRAME / 2, overlap: false };
      if (r < bestRms) { bestRms = r; best = f + FRAME / 2; }
    }
  }
  return { cut: a.length >= MAX_BATCH ? best : 0, overlap: true };
}

// compact shortens every silence longer than LONG_GAP frames to KEEP_GAP
// frames, keeping its edges, so there is less audio and less to hallucinate on.
function compact(a: Float32Array): Float32Array {
  const parts: Float32Array[] = [];
  let gap = -1; // start of the current silent run
  const flush = (end: number) => {
    if (gap < 0) return;
    const len = end - gap;
    if (len > LONG_GAP * FRAME) {
      const half = (KEEP_GAP * FRAME) / 2;
      parts.push(a.subarray(gap, gap + half), a.subarray(end - half, end));
    } else parts.push(a.subarray(gap, end));
    gap = -1;
  };
  let f = 0;
  for (; f + FRAME <= a.length; f += FRAME) {
    if (rms(a, f) < SILENCE) { if (gap < 0) gap = f; continue; }
    flush(f);
    parts.push(a.subarray(f, f + FRAME));
  }
  flush(f);
  parts.push(a.subarray(f));
  const out = new Float32Array(parts.reduce((n, p) => n + p.length, 0));
  let o = 0;
  for (const p of parts) { out.set(p, o); o += p.length; }
  return out;
}

// useDictation records the microphone and uploads short batches (cut on pauses, 6 s at most) to the server
// while the user speaks; the partial text arrives by polling.
export function useDictation() {
  const [recording, setRecording] = useState(false);
  const [busy, setBusy] = useState(false);
  const [partial, setPartial] = useState("");
  const [error, setError] = useState<unknown>(null);
  const meter = useRef<HTMLSpanElement>(null);
  const reservation = useRef<(() => void) | null>(null);
  const r = useRef<{ id: string; seq: number; buf: Float32Array; uploads: Promise<unknown>; stop: () => void; poll: number } | null>(null);

  // upload sends a batch without its long silences; a batch with no voice is
  // dropped and does not use a seq.
  const upload = (samples: Float32Array) => {
    const s = r.current!;
    if (!voiced(samples)) return;
    samples = compact(samples);
    const seq = s.seq++;
    s.uploads = s.uploads.then(() => call<DictationState>("POST", `/${s.id}/chunks?seq=${seq}`, wav(samples), false)).catch((e) => { setError(e); throw e; });
  };

  const start = async () => {
    setError(null); setPartial("");
    if (!window.isSecureContext || !navigator.mediaDevices) {
      setError(new Error(`El navegador solo permite el micrófono en HTTPS o localhost; esta página es ${location.origin}.`));
      return;
    }
    const releaseMic = reserveDictation();
    if (!releaseMic) return;
    prepareDictationSound();
    reservation.current = releaseMic;
    const release = () => {
      if (reservation.current === releaseMic) reservation.current = null;
      releaseMic();
    };
    let stream: MediaStream;
    try {
      stream = await openDictationMicrophone();
    } catch (e) {
      const name = e instanceof DOMException ? e.name : "";
      release();
      setError(new Error(name === "NotFoundError" ? "No se encuentra ningún micrófono." : `No hay permiso para usar el micrófono (${name || String(e)}).`));
      return;
    }
    try {
      if (reservation.current !== releaseMic) { stream.getTracks().forEach((track) => track.stop()); return; }
      const { id } = await call<DictationState>("POST", "");
      if (reservation.current !== releaseMic) { stream.getTracks().forEach((track) => track.stop()); return; }
      const ctx = new AudioContext();
      const source = ctx.createMediaStreamSource(stream);
      const proc = ctx.createScriptProcessor(4096, 1, 1);
      const ratio = ctx.sampleRate / RATE;
      const updateMeter = createDictationMeter(meter);
      proc.onaudioprocess = (e) => {
        const s = r.current;
        if (!s) return;
        const input = e.inputBuffer.getChannelData(0);
        updateMeter(input, ctx.currentTime);
        const out = new Float32Array(Math.floor(input.length / ratio));
        for (let i = 0; i < out.length; i++) out[i] = input[Math.floor(i * ratio)];
        const merged = new Float32Array(s.buf.length + out.length);
        merged.set(s.buf); merged.set(out, s.buf.length);
        s.buf = merged;
        const { cut, overlap } = cutPoint(s.buf);
        if (cut) { upload(s.buf.slice(0, cut)); s.buf = s.buf.slice(overlap ? Math.max(0, cut - OVERLAP) : cut); }
      };
      source.connect(proc); proc.connect(ctx.destination);
      const poll = window.setInterval(() => { call<DictationState>("GET", `/${id}`).then((st) => setPartial(st.text)).catch(() => {}); }, 1000);
      r.current = { id, seq: 0, buf: new Float32Array(0), uploads: Promise.resolve(), poll, stop: () => { if (proc.onaudioprocess) { proc.onaudioprocess = null; proc.disconnect(); source.disconnect(); stream.getTracks().forEach((t) => t.stop()); void ctx.close().catch(() => {}); } } };
      setRecording(true);
      playDictationSound("start");
    } catch (e) {
      stream.getTracks().forEach((t) => t.stop());
      release();
      setError(e);
    }
  };

  // stop flushes the last batch and finishes: insert returns the text, send
  // hands it to the backend for target.
  const stop = async (target?: DictationTarget): Promise<string> => {
    const s = r.current;
    if (!s) return "";
    s.stop(); window.clearInterval(s.poll);
    playDictationSound("stop");
    setRecording(false); setBusy(true);
    try {
      upload(s.buf);
      await s.uploads;
      const st = await call<DictationState>("POST", `/${s.id}/finish`, JSON.stringify(target ? { mode: "send", target } : { mode: "insert" }));
      return target ? "" : st.text;
    } catch (e) {
      setError(e);
      return "";
    } finally {
      reservation.current?.();
      reservation.current = null;
      r.current = null; setBusy(false); setPartial("");
    }
  };

  useEffect(() => () => {
    const s = r.current;
    if (s) { s.stop(); window.clearInterval(s.poll); }
    reservation.current?.();
    reservation.current = null;
  }, []);
  return { recording, busy, partial, error, start, stop, meter };
}

// DictationInput is the test version: a box, a microphone and Send. While
// dictating, Send becomes an arrow that hands the dictation to the backend.
export function DictationInput({ target, onSubmit, onSent, placeholder }: { target: DictationTarget; onSubmit: (text: string) => void; onSent?: () => void; placeholder?: string }) {
  const [text, setText] = useState("");
  const d = useDictation();
  const input = useRef<HTMLTextAreaElement>(null);
  const activeElsewhere = useDictationInProgress() && !d.recording;
  const mic = async () => {
    if (!d.recording) {
      input.current?.focus({ preventScroll: true });
      return d.start();
    }
    const t = await d.stop();
    if (t) setText((prev) => (prev.trim() ? `${prev.trimEnd()} ${t}` : t));
  };
  useDictationShortcut((event, context) => {
    const field = input.current;
    if (!field) return false;
    if (context === "active") {
      if (!d.recording && !d.busy) return false;
      if (d.recording) void mic();
      return true;
    }
    if (context !== "editor" || activeElsewhere || d.busy || hasDictationPopupOutside(field) || !(event.target instanceof Element)) return false;
    const target = event.target;
    if (target !== field && (isTextInput(target) || target.closest("[data-dictation-panel]") !== field.closest("[data-dictation-panel]"))) return false;
    void mic();
    return true;
  });
  const send = async () => {
    if (d.recording && text.trim()) {
      // The backend only knows the dictated text; with typed text too, wait and send both here.
      const t = await d.stop();
      onSubmit(`${text.trim()} ${t}`.trim());
      setText("");
      return;
    }
    if (d.recording) {
      await d.stop(target);
      setText("");
      onSent?.();
      return;
    }
    if (text.trim()) { onSubmit(text.trim()); setText(""); }
  };
  return (
    <div data-dictation-panel className="space-y-1 rounded-lg border border-dashed border-slate-300 p-2 dark:border-border">
      <p className="text-xs text-slate-500 dark:text-muted">Prueba de dictado</p>
      <Textarea ref={input} value={d.recording && d.partial ? `${text}${text ? " " : ""}${d.partial}` : text} readOnly={d.recording || d.busy} onChange={(e) => setText(e.target.value)} placeholder={placeholder ?? "Escribe o pulsa el micrófono para dictar"} className="min-h-20" />
      <ErrorText error={d.error} />
      <div className="flex items-center justify-end gap-2">
        {d.busy && <span className="text-xs text-slate-500 dark:text-muted">Transcribiendo…</span>}
        <DictationMic dictation={{
          phase: d.recording ? "recording" : d.busy ? "finishing" : "idle",
          active: d.recording || d.busy,
          model: { name: "Whisper" },
          meter: d.meter,
          toggle: mic,
        }} />
        <Button type="button" disabled={d.busy || activeElsewhere || (!d.recording && !text.trim())} onClick={send} title={d.recording ? "Enviar al terminar el dictado" : "Enviar"}>
          {d.recording ? "→" : "Enviar"}
        </Button>
      </div>
    </div>
  );
}
