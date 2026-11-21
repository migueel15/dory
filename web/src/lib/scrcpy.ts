// Client of a device stream: a WebSocket to the node that carries scrcpy's
// H.264 video, decoded with WebCodecs onto a canvas.
export type StreamStatus = "connecting" | "live" | "closed" | "error";
export type StreamMeta = { name: string; codec: string; width: number; height: number };

export function webCodecsProblem(): string | null {
  if (typeof VideoDecoder === "undefined") return window.isSecureContext ? "Este navegador no admite WebCodecs." : "El vídeo del dispositivo necesita HTTPS o localhost (WebCodecs no está disponible en un contexto no seguro).";
  return null;
}

export type StreamOptions = { node: string; serial: string; maxSize?: number; bitrate?: number };

export function streamUrl(o: StreamOptions): string {
  const q = new URLSearchParams();
  if (o.maxSize) q.set("max_size", String(o.maxSize));
  if (o.bitrate) q.set("bitrate", String(o.bitrate));
  const proto = location.protocol === "https:" ? "wss" : "ws";
  return `${proto}://${location.host}/api/nodes/${o.node}/devices/${encodeURIComponent(o.serial)}/stream?${q}`;
}

// avc1 codec string from the SPS in scrcpy's config packet (Annex B).
function codecOf(data: Uint8Array): string {
  for (let i = 0; i + 4 < data.length; i++) {
    const start = data[i] === 0 && data[i + 1] === 0 && (data[i + 2] === 1 || (data[i + 2] === 0 && data[i + 3] === 1));
    if (!start) continue;
    const j = i + (data[i + 2] === 1 ? 3 : 4);
    if ((data[j] & 0x1f) === 7) return "avc1." + [data[j + 1], data[j + 2], data[j + 3]].map((b) => b.toString(16).padStart(2, "0")).join("");
  }
  return "avc1.42e01f";
}

export class DeviceStream {
  private ws: WebSocket | null = null;
  private decoder: VideoDecoder | null = null;
  private ctx: CanvasRenderingContext2D | null;
  private gotKey = false;
  private ts = 0;
  private stopped = false;
  private retry: ReturnType<typeof setTimeout> | null = null;
  // size of the last decoded frame: touch coordinates are expressed in it
  size = { width: 0, height: 0 };

  constructor(private canvas: HTMLCanvasElement, private o: StreamOptions, private on: { status: (s: StreamStatus, message?: string) => void; meta?: (m: StreamMeta) => void; size?: (w: number, h: number) => void }) {
    this.ctx = canvas.getContext("2d");
  }

  start() {
    this.stopped = false;
    this.connect();
  }

  stop() {
    this.stopped = true;
    if (this.retry) clearTimeout(this.retry);
    this.ws?.close();
    this.ws = null;
    this.reset();
  }

  private reset() {
    try { if (this.decoder && this.decoder.state !== "closed") this.decoder.close(); } catch { /* already closed */ }
    this.decoder = null;
    this.gotKey = false;
  }

  private connect() {
    this.on.status("connecting");
    const ws = new WebSocket(streamUrl(this.o));
    ws.binaryType = "arraybuffer";
    this.ws = ws;
    let failed = "";
    ws.onmessage = (e) => {
      if (typeof e.data === "string") {
        const m = JSON.parse(e.data);
        if (m.type === "error") { failed = m.message; this.on.status("error", m.message); }
        else if (m.type === "meta") this.on.meta?.(m);
        else if (m.type === "closed") this.on.status("closed");
        return;
      }
      this.frame(new Uint8Array(e.data as ArrayBuffer));
    };
    ws.onclose = () => {
      if (this.ws !== ws) return;
      this.reset();
      if (this.stopped) return;
      if (!failed) this.on.status("closed");
      // The device may be rebooting or the cable moved: keep trying.
      this.retry = setTimeout(() => this.connect(), 3000);
    };
  }

  private frame(msg: Uint8Array) {
    const kind = msg[0], data = msg.subarray(1);
    if (kind === 0) {
      this.reset();
      this.decoder = new VideoDecoder({
        output: (f) => {
          if (this.canvas.width !== f.displayWidth || this.canvas.height !== f.displayHeight) {
            this.canvas.width = f.displayWidth; this.canvas.height = f.displayHeight;
            this.size = { width: f.displayWidth, height: f.displayHeight };
            this.on.size?.(f.displayWidth, f.displayHeight);
          }
          this.ctx?.drawImage(f, 0, 0);
          f.close();
          this.on.status("live");
        },
        error: () => { this.reset(); },
      });
      try { this.decoder.configure({ codec: codecOf(data), optimizeForLatency: true }); } catch { this.on.status("error", "El navegador no puede decodificar este vídeo."); return; }
      // The config (SPS/PPS) travels with the next key frame.
      this.pending = data.slice();
      return;
    }
    if (!this.decoder || this.decoder.state !== "configured") return;
    if (kind === 1) {
      this.gotKey = true;
      let chunk = data;
      if (this.pending) { chunk = new Uint8Array(this.pending.length + data.length); chunk.set(this.pending); chunk.set(data, this.pending.length); this.pending = null; }
      this.decode("key", chunk);
    } else if (this.gotKey) this.decode("delta", data);
  }
  private pending: Uint8Array | null = null;

  private decode(type: "key" | "delta", data: Uint8Array) {
    try { this.decoder!.decode(new EncodedVideoChunk({ type, timestamp: (this.ts += 16000), data })); } catch { /* wait for the next key frame */ }
  }

  send(msg: Uint8Array) {
    if (this.ws?.readyState === WebSocket.OPEN) this.ws.send(msg);
  }
}
