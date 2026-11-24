// Client of a thread's terminal: a binary WebSocket to the node that runs the
// shell. It has no UI: the React panel mounts it on an xterm, and the mobile
// app can reuse it. Output arrives as raw bytes; input leaves as binary
// frames and the size as a small JSON text frame.
export type TerminalStatus = "connecting" | "live" | "exited" | "gone";

export type TerminalInfo = { id: string; name: string; cwd: string; running: boolean };

export type TerminalHandlers = {
  /** Output bytes. */
  data: (chunk: Uint8Array) => void;
  /** The connection (re)opened: the node now replays the recent output, so the screen starts over. */
  reset: () => void;
  status: (s: TerminalStatus) => void;
};

const base = (threadId: string) => `/api/threads/${encodeURIComponent(threadId)}/terminals`;

async function call<T>(method: string, path: string, body?: unknown): Promise<T> {
  const res = await fetch(path, {
    method, credentials: "include",
    headers: body === undefined ? undefined : { "Content-Type": "application/json" },
    body: body === undefined ? undefined : JSON.stringify(body),
  });
  if (!res.ok) throw Object.assign(new Error(`terminal ${method} ${res.status}`), { status: res.status });
  return res.status === 204 ? (undefined as T) : res.json();
}

export const terminalApi = {
  list: (threadId: string) => call<TerminalInfo[]>("GET", base(threadId)),
  open: (threadId: string, size?: { cols: number; rows: number }, name?: string) => call<TerminalInfo>("POST", base(threadId), { name, ...size }),
  rename: (threadId: string, id: string, name: string) => call<TerminalInfo>("PATCH", `${base(threadId)}/${id}`, { name }),
  close: (threadId: string, id: string) => call<void>("DELETE", `${base(threadId)}/${id}`),
};

const encoder = new TextEncoder();

export class TerminalClient {
  private ws: WebSocket | null = null;
  private stopped = false;
  private opened = false;
  private attempts = 0;
  private retry: ReturnType<typeof setTimeout> | null = null;
  private size = { cols: 80, rows: 24 };

  constructor(private threadId: string, private id: string, private on: TerminalHandlers) {}

  start() {
    this.stopped = false;
    this.connect();
  }

  stop() {
    this.stopped = true;
    if (this.retry) clearTimeout(this.retry);
    this.ws?.close();
    this.ws = null;
  }

  /** send writes keys typed by the user; a string is UTF-8, a binary string (mouse reports) is raw bytes. */
  send(data: string, binary = false) {
    if (this.ws?.readyState !== WebSocket.OPEN) return;
    if (binary) {
      const bytes = new Uint8Array(data.length);
      for (let i = 0; i < data.length; i++) bytes[i] = data.charCodeAt(i) & 0xff;
      this.ws.send(bytes);
    } else this.ws.send(encoder.encode(data));
  }

  resize(cols: number, rows: number) {
    this.size = { cols, rows };
    if (this.ws?.readyState === WebSocket.OPEN) this.ws.send(JSON.stringify({ type: "resize", cols, rows }));
  }

  private connect() {
    this.on.status("connecting");
    const proto = location.protocol === "https:" ? "wss" : "ws";
    const ws = new WebSocket(`${proto}://${location.host}${base(this.threadId)}/${this.id}/ws`);
    ws.binaryType = "arraybuffer";
    this.ws = ws;
    this.opened = false;
    ws.onopen = () => {
      this.opened = true;
      this.attempts = 0;
      this.on.reset();
      this.on.status("live");
      ws.send(JSON.stringify({ type: "resize", ...this.size }));
    };
    ws.onmessage = (e) => { if (e.data instanceof ArrayBuffer) this.on.data(new Uint8Array(e.data)); };
    ws.onclose = (e) => {
      if (this.stopped || this.ws !== ws) return;
      // 1000 is the node saying the shell exited.
      if (this.opened && e.code === 1000) { this.on.status("exited"); return; }
      void this.lost();
    };
  }

  // The socket dropped or never opened: the session may be gone for good, or
  // the node may just be away for a moment.
  private async lost() {
    try {
      const all = await terminalApi.list(this.threadId);
      const mine = all.find((t) => t.id === this.id);
      if (!mine) { this.on.status("gone"); return; }
      if (!mine.running) { this.on.status("exited"); return; }
    } catch (e) {
      if ((e as { status?: number }).status === 404) { this.on.status("gone"); return; }
    }
    if (this.stopped) return;
    this.on.status("connecting");
    this.retry = setTimeout(() => this.connect(), Math.min(5000, 300 * 2 ** this.attempts++));
  }
}
