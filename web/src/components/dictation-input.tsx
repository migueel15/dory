import { useEffect, useRef, useState } from "react";
import { ApiError } from "../api/client";
import type { components } from "../api/schema";
import { Button, ErrorText, Textarea } from "./ui";

type DictationState = components["schemas"]["DictationState"];
export type DictationTarget = { kind: "capture"; project_id?: string } | { kind: "define"; item_id: string };

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
    let stream: MediaStream;
    try {
      stream = await navigator.mediaDevices.getUserMedia({ audio: { channelCount: 1, echoCancellation: true, noiseSuppression: true } });
    } catch (e) {
      const name = e instanceof DOMException ? e.name : "";
      setError(new Error(name === "NotFoundError" ? "No se encuentra ningún micrófono." : `No hay permiso para usar el micrófono (${name || String(e)}).`));
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
        const { cut, overlap } = cutPoint(s.buf);
        if (cut) { upload(s.buf.slice(0, cut)); s.buf = s.buf.slice(overlap ? Math.max(0, cut - OVERLAP) : cut); }
      };
      source.connect(proc); proc.connect(ctx.destination);
      const poll = window.setInterval(() => { call<DictationState>("GET", `/${id}`).then((st) => setPartial(st.text)).catch(() => {}); }, 1000);
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
      upload(s.buf);
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
