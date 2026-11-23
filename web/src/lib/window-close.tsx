import { X } from "@phosphor-icons/react";

declare global {
  interface Window { doryDesktop?: { platform: string; setZoom?: (factor: number) => void; browser?: DesktopBrowser } }
  // DesktopBrowser is the bridge to the native views of the thread browser
  // (desktop/preload.js); bounds are CSS pixels of the interface.
  type Bounds = { x: number; y: number; width: number; height: number };
  interface DesktopBrowser {
    show(tab: string, url: string, bounds: Bounds): Promise<void>;
    bounds(tab: string, bounds: Bounds, visible: boolean): Promise<void>;
    hide(tab: string): Promise<void>;
    // Optional: older desktop builds lack them.
    park?(tab: string): Promise<void>;
    focusUI?(): Promise<void>;
    onKey?(cb: (tab: string, key: string) => void): () => void;
    cdp(tab: string, method: string, params: unknown): Promise<unknown>;
    onEvent(cb: (tab: string, method: string, params: unknown) => void): () => void;
    onOpen(cb: (tab: string, url: string) => void): () => void;
    onGone(cb: (tab: string) => void): () => void;
    tunnelListen(port: number): Promise<boolean>;
    tunnelWrite(id: number, data: ArrayBuffer): void;
    tunnelEnd(id: number): void;
    onTunnelOpen(cb: (id: number, port: number) => void): () => void;
    onTunnelData(cb: (id: number, data: Uint8Array) => void): () => void;
    onTunnelClose(cb: (id: number) => void): () => void;
  }
}

// The desktop app on Linux may run under window managers without title bars
// (tiling ones), so the view carries its own close button. Closing hides the
// window to the tray, as the native button does elsewhere.
export const hasWindowClose = window.doryDesktop?.platform === "linux";

// WindowClose sits fixed at the top right of the main view, always in the
// same place; views leave room for it.
export function WindowClose() {
  if (!hasWindowClose) return null;
  return (
    <button type="button" onClick={() => window.close()} title="Cerrar la ventana" aria-label="Cerrar la ventana"
      className="fixed right-[1.0625rem] top-[1.0625rem] z-40 hidden size-8 place-items-center rounded-lg text-slate-600 hover:bg-slate-100 sm:grid dark:text-text-secondary dark:hover:bg-raised">
      <X weight="regular" className="size-4" />
    </button>
  );
}
