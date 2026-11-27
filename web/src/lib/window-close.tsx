import { X } from "@phosphor-icons/react";

declare global {
  interface Window { doryDesktop?: { platform: string; setZoom?: (factor: number) => void; hide?: () => void; browser?: DesktopBrowser } }
  // DesktopBrowser is the bridge to the native views of the thread browser
  // (desktop/preload.js); bounds are CSS pixels of the interface.
  type CookieImportSettings = { source: string; auto: boolean };
  type Bounds = { x: number; y: number; width: number; height: number };
  interface DesktopBrowser {
    show(tab: string, url: string, bounds: Bounds): Promise<void>;
    bounds(tab: string, bounds: Bounds, visible: boolean): Promise<void>;
    hide(tab: string): Promise<void>;
    // Optional: older desktop builds lack them.
    park?(tab: string): Promise<void>;
    focusUI?(): Promise<void>;
    importCookies?(source?: string): Promise<{ browser: string; imported: number; failed: number }>;
    importSources?(): Promise<{ sources: { id: string; name: string }[]; default: string; settings: CookieImportSettings }>;
    importSettings?(s: CookieImportSettings): Promise<CookieImportSettings>;
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

// windowCloseInset reserves horizontal room in top-right page headers so their
// actions do not collide with the floating WindowClose button on Linux desktop.
export const windowCloseInset = hasWindowClose ? "sm:pr-12" : "";

// WindowClose sits at the far right of the desktop top bar.
export function WindowClose() {
  if (!hasWindowClose) return null;
  return (
    <button type="button" onClick={() => (window.doryDesktop?.hide ? window.doryDesktop.hide() : window.close())} title="Cerrar la ventana" aria-label="Cerrar la ventana"
      className="m-0.5 grid size-7 place-items-center rounded-full bg-slate-200/80 text-slate-600 transition-[background-color,color,transform] duration-150 hover:bg-slate-300/80 hover:text-slate-900 active:scale-95 dark:bg-raised dark:text-text-secondary dark:hover:bg-hover dark:hover:text-white">
      <X weight="bold" className="size-3.5" />
    </button>
  );
}
