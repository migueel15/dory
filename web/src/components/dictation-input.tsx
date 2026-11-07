import { useEffect, useRef, useState } from "react";
import { ApiError } from "../api/client";
import type { components } from "../api/schema";
import { Button, ErrorText, Textarea } from "./ui";

type DictationState = components["schemas"]["DictationState"];
export type DictationTarget = { kind: "capture"; project_id?: string } | { kind: "define"; item_id: string };

const RATE = 16000;
const MIN_BATCH = 8 * RATE; // look for a silence from 8 s on
const MAX_BATCH = 12 * RATE; // cut anyway at 12 s
const OVERLAP = RATE / 2; // 0.5 s repeated at the start of the next batch
const FRAME = RATE / 10; // 100 ms frames to measure energy
const SILENCE = 0.01; // RMS below this counts as silence

async function call<T>(method: string, path: string, body?: BodyInit, json = true): Promise<T> {
  const res = await fetch(`/api/dictation${path}`, { method, credentials: "same-origin", body, headers: body && json ? { "Content-Type": "application/json" } : body ? { "Content-Type": "audio/wav" } : undefined });
  if (!res.ok) {
    const e = await res.json().catch(() => null);
    throw new ApiError(res.status, e?.error?.message ?? res.statusText);
  }
  return res.json();
}

// wav encodes 16 kHz mono float samples as a self-contained 16-bit PCM WAV.
function wav(samples: Float32Array): Blob {
  const buf = new ArrayBuffer(44 + samples.length * 2);
  const v = new DataView(buf);
  const str = (o: number, s: string) => { for (let i = 0; i < s.length; i++) v.setUint8(o + i, s.charCodeAt(i)); };
  str(0, "RIFF"); v.setUint32(4, 36 + samples.length * 2, true); str(8, "WAVE"); str(12, "fmt ");
  v.setUint32(16, 16, true); v.setUint16(20, 1, true); v.setUint16(22, 1, true); v.setUint32(24, RATE, true);
  v.setUint32(28, RATE * 2, true); v.setUint16(32, 2, true); v.setUint16(34, 16, true); str(36, "data"); v.setUint32(40, samples.length * 2, true);
  for (let i = 0; i < samples.length; i++) { const s = Math.max(-1, Math.min(1, samples[i])); v.setInt16(44 + i * 2, s < 0 ? s * 0x8000 : s * 0x7fff, true); }
  return new Blob([buf], { type: "audio/wav" });
}

function rms(a: Float32Array, from: number) {
  let sum = 0;
  for (let i = from; i < from + FRAME; i++) sum += a[i] * a[i];
  return Math.sqrt(sum / FRAME);
}

// cutPoint returns where to end the batch, or 0 to keep recording: the first
// silent frame after MIN_BATCH, or the quietest frame once MAX_BATCH is reached.
function cutPoint(a: Float32Array): number {
  if (a.length < MIN_BATCH + FRAME) return 0;
  let best = MIN_BATCH, bestRms = Infinity;
  for (let f = MIN_BATCH; f + FRAME <= Math.min(a.length, MAX_BATCH); f += FRAME) {
    const r = rms(a, f);
    if (r < SILENCE) return f + FRAME / 2;
    if (r < bestRms) { bestRms = r; best = f + FRAME / 2; }
  }
  return a.length >= MAX_BATCH ? best : 0;
}

// useDictation records the microphone and uploads ~10 s batches to the server
// while the user speaks; the partial text arrives by polling.
export function useDictation() {
  const [recording, setRecording] = useState(false);
  const [busy, setBusy] = useState(false);
  const [partial, setPartial] = useState("");
  const [error, setError] = useState<unknown>(null);
  const r = useRef<{ id: string; seq: number; buf: Float32Array; uploads: Promise<unknown>; stop: () => void; poll: number } | null>(null);

  const upload = (samples: Float32Array) => {
    const s = r.current!;
    const seq = s.seq++;
    s.uploads = s.uploads.then(() => call<DictationState>("POST", `/${s.id}/chunks?seq=${seq}`, wav(samples), false)).catch((e) => { setError(e); throw e; });
  };

  const start = async () => {
    setError(null); setPartial("");
    let stream: MediaStream;
    try {
      stream = await navigator.mediaDevices.getUserMedia({ audio: { channelCount: 1, echoCancellation: true, noiseSuppression: true } });
    } catch {
      setError(new Error("No hay permiso para usar el micrófono."));
      return;
    }
    try {
      const { id } = await call<DictationState>("POST", "");
      const ctx = new AudioContext();
      const source = ctx.createMediaStreamSource(stream);
      const proc = ctx.createScriptProcessor(4096, 1, 1);
      const ratio = ctx.sampleRate / RATE;
      proc.onaudioprocess = (e) => {
        const s = r.current;
        if (!s) return;
        const input = e.inputBuffer.getChannelData(0);
        const out = new Float32Array(Math.floor(input.length / ratio));
        for (let i = 0; i < out.length; i++) out[i] = input[Math.floor(i * ratio)];
        const merged = new Float32Array(s.buf.length + out.length);
        merged.set(s.buf); merged.set(out, s.buf.length);
        s.buf = merged;
        const cut = cutPoint(s.buf);
        if (cut) { upload(s.buf.slice(0, cut)); s.buf = s.buf.slice(Math.max(0, cut - OVERLAP)); }
      };
      source.connect(proc); proc.connect(ctx.destination);
      const poll = window.setInterval(() => { call<DictationState>("GET", `/${id}`).then((st) => setPartial(st.text)).catch(() => {}); }, 2000);
      r.current = { id, seq: 0, buf: new Float32Array(0), uploads: Promise.resolve(), poll, stop: () => { proc.disconnect(); source.disconnect(); stream.getTracks().forEach((t) => t.stop()); ctx.close(); } };
      setRecording(true);
    } catch (e) {
      stream.getTracks().forEach((t) => t.stop());
      setError(e);
    }
  };

  // stop flushes the last batch and finishes: insert returns the text, send
  // hands it to the backend for target.
  const stop = async (target?: DictationTarget): Promise<string> => {
    const s = r.current;
    if (!s) return "";
    s.stop(); window.clearInterval(s.poll);
    setRecording(false); setBusy(true);
    try {
      if (s.buf.length > RATE / 3) upload(s.buf);
      await s.uploads;
      const st = await call<DictationState>("POST", `/${s.id}/finish`, JSON.stringify(target ? { mode: "send", target } : { mode: "insert" }));
      return target ? "" : st.text;
    } catch (e) {
      setError(e);
      return "";
    } finally {
      r.current = null; setBusy(false); setPartial("");
    }
  };

  useEffect(() => () => { const s = r.current; if (s) { s.stop(); window.clearInterval(s.poll); } }, []);
  return { recording, busy, partial, error, start, stop };
}

// DictationInput is the test version: a box, a microphone and Send. While
// dictating, Send becomes an arrow that hands the dictation to the backend.
export function DictationInput({ target, onSubmit, onSent, placeholder }: { target: DictationTarget; onSubmit: (text: string) => void; onSent?: () => void; placeholder?: string }) {
  const [text, setText] = useState("");
  const d = useDictation();
  const mic = async () => {
    if (!d.recording) return d.start();
    const t = await d.stop();
    if (t) setText((prev) => (prev.trim() ? `${prev.trimEnd()} ${t}` : t));
  };
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
    <div className="space-y-1 rounded-lg border border-dashed border-slate-300 p-2 dark:border-border">
      <p className="text-xs text-slate-500 dark:text-muted">Prueba de dictado</p>
      <Textarea value={d.recording && d.partial ? `${text}${text ? " " : ""}${d.partial}` : text} readOnly={d.recording || d.busy} onChange={(e) => setText(e.target.value)} placeholder={placeholder ?? "Escribe o pulsa el micrófono para dictar"} className="min-h-20" />
      <ErrorText error={d.error} />
      <div className="flex items-center justify-end gap-2">
        {d.busy && <span className="text-xs text-slate-500 dark:text-muted">Transcribiendo…</span>}
        <Button type="button" variant="ghost" disabled={d.busy} onClick={mic} aria-label={d.recording ? "Parar dictado" : "Dictar"} title={d.recording ? "Parar y pegar el texto" : "Dictar"}>
          {d.recording ? <span className="motion-safe:animate-pulse text-red-600">● Parar</span> : "🎤"}
        </Button>
        <Button type="button" disabled={d.busy || (!d.recording && !text.trim())} onClick={send} title={d.recording ? "Enviar al terminar el dictado" : "Enviar"}>
          {d.recording ? "→" : "Enviar"}
        </Button>
      </div>
    </div>
  );
}
