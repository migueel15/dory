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
  // The newest decoded frame waits here and is painted once per screen refresh,
  // so a burst of frames never costs more than one draw.
  private latest: VideoFrame | null = null;
  private raf = 0;
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

  private paint() {
    this.raf = 0;
    const f = this.latest;
    this.latest = null;
    if (!f) return;
    if (this.canvas.width !== f.displayWidth || this.canvas.height !== f.displayHeight) {
      this.canvas.width = f.displayWidth; this.canvas.height = f.displayHeight;
      this.size = { width: f.displayWidth, height: f.displayHeight };
      this.on.size?.(f.displayWidth, f.displayHeight);
    }
    this.ctx?.drawImage(f, 0, 0);
    f.close();
    this.on.status("live");
  }

  private reset() {
    if (this.raf) { cancelAnimationFrame(this.raf); this.raf = 0; }
    this.latest?.close();
    this.latest = null;
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
          this.latest?.close();
          this.latest = f;
          if (!this.raf) this.raf = requestAnimationFrame(() => this.paint());
        },
        error: () => { this.reset(); },
      });
      try { this.decoder.configure({ codec: codecOf(data), optimizeForLatency: true, hardwareAcceleration: "prefer-hardware" }); } catch { this.on.status("error", "El navegador no puede decodificar este vídeo."); return; }
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

  // --- control: scrcpy control messages, big endian ---
  private msg(size: number, fill: (v: DataView) => void) {
    const b = new Uint8Array(size);
    fill(new DataView(b.buffer));
    this.send(b);
  }

  // touch sends a pointer event in device pixels; action 0 down, 1 up, 2 move.
  touch(action: 0 | 1 | 2, x: number, y: number, pointer = 0) {
    const { width, height } = this.size;
    if (!width) return;
    this.msg(32, (v) => {
      v.setUint8(0, 2); v.setUint8(1, action);
      v.setBigInt64(2, BigInt(pointer === 0 ? -2 : pointer)); // -2 is scrcpy's generic finger
      v.setInt32(10, Math.round(x)); v.setInt32(14, Math.round(y));
      v.setUint16(18, width); v.setUint16(20, height);
      v.setUint16(22, action === 1 ? 0 : 0xffff);
      v.setInt32(24, action === 2 ? 0 : 1); v.setInt32(28, action === 1 ? 0 : 1);
    });
  }

  // scroll sends a wheel step; positive vertical scrolls up.
  scroll(x: number, y: number, h: number, vert: number) {
    const { width, height } = this.size;
    if (!width) return;
    const fix = (n: number) => Math.round(Math.max(-1, Math.min(1, n / 16)) * 32767);
    this.msg(21, (v) => {
      v.setUint8(0, 3); v.setInt32(1, Math.round(x)); v.setInt32(5, Math.round(y));
      v.setUint16(9, width); v.setUint16(11, height);
      v.setInt16(13, fix(h)); v.setInt16(15, fix(vert)); v.setInt32(17, 0);
    });
  }

  keycode(code: number) {
    for (const action of [0, 1]) this.msg(14, (v) => { v.setUint8(0, 0); v.setUint8(1, action); v.setInt32(2, code); v.setInt32(6, 0); v.setInt32(10, 0); });
  }

  text(text: string) {
    const bytes = new TextEncoder().encode(text);
    const b = new Uint8Array(5 + bytes.length);
    new DataView(b.buffer).setUint32(1, bytes.length);
    b[0] = 1; b.set(bytes, 5);
    this.send(b);
  }

  // back goes back, or wakes the screen when it is off.
  back() { for (const action of [0, 1]) this.msg(2, (v) => { v.setUint8(0, 4); v.setUint8(1, action); }); }
  displayPower(on: boolean) { this.msg(2, (v) => { v.setUint8(0, 10); v.setUint8(1, on ? 1 : 0); }); }
}

// Android keycodes used by the bar and the keyboard.
export const Key = { home: 3, back: 4, power: 26, volUp: 24, volDown: 25, appSwitch: 187, enter: 66, del: 67, tab: 61, esc: 111, up: 19, down: 20, left: 21, right: 22, fwdDel: 112, space: 62 };
const byName: Record<string, number> = { Enter: Key.enter, Backspace: Key.del, Tab: Key.tab, Escape: Key.esc, ArrowUp: Key.up, ArrowDown: Key.down, ArrowLeft: Key.left, ArrowRight: Key.right, Delete: Key.fwdDel };
export const keyFor = (name: string) => byName[name];
