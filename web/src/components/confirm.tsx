import { useEffect } from "react";
import { createRoot } from "react-dom/client";
import { Button } from "./ui";

// askConfirm replaces window.confirm with a dialog in the app's style. It
// resolves true when the action is confirmed and false otherwise.
export function askConfirm(message: string, action = "Aceptar"): Promise<boolean> {
  return new Promise((resolve) => {
    const host = document.createElement("div");
    document.body.appendChild(host);
    const root = createRoot(host);
    const done = (ok: boolean) => { root.unmount(); host.remove(); resolve(ok); };
    root.render(<ConfirmDialog message={message} action={action} done={done} />);
  });
}

function ConfirmDialog({ message, action, done }: { message: string; action: string; done: (ok: boolean) => void }) {
  useEffect(() => {
    const previous = document.activeElement as HTMLElement | null;
    // Capture first so an open modal underneath does not close on the same Escape.
    const onKey = (e: KeyboardEvent) => { if (e.key === "Escape") { e.stopImmediatePropagation(); done(false); } };
    window.addEventListener("keydown", onKey, true);
    return () => { window.removeEventListener("keydown", onKey, true); previous?.focus(); };
  }, [done]);
  return (
    <div className="fixed inset-0 z-[110] flex items-center justify-center bg-black/40 p-2 sm:p-4" onClick={() => done(false)}>
      <div role="alertdialog" aria-modal="true" aria-label={message} className="w-full max-w-sm space-y-4 rounded-lg bg-white p-4 shadow-lg dark:bg-surface" onClick={(e) => e.stopPropagation()}>
        <p className="text-sm">{message}</p>
        <div className="flex justify-end gap-2">
          <Button type="button" variant="ghost" onClick={() => done(false)}>Cancelar</Button>
          <Button autoFocus type="button" variant="danger" onClick={() => done(true)}>{action}</Button>
        </div>
      </div>
    </div>
  );
}
