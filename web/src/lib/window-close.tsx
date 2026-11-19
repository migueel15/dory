import { X } from "@phosphor-icons/react";

declare global {
  interface Window { doryDesktop?: { platform: string; setZoom?: (factor: number) => void } }
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
