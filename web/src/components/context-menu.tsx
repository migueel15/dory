import { useLayoutEffect, useRef, useState, type ReactNode } from "react";
import { ArrowLeft } from "@phosphor-icons/react";
import { createPortal } from "react-dom";

export type ContextAction = { label: string; run?: () => void; content?: ReactNode; children?: ContextAction[] };

export function ContextMenu({ x, y, actions, onClose }: { x: number; y: number; actions: ContextAction[]; onClose: () => void }) {
  const ref = useRef<HTMLDivElement>(null);
  const [submenu, setSubmenu] = useState<ContextAction[] | null>(null);
  const entries = submenu ?? actions;
  useLayoutEffect(() => {
    const node = ref.current!;
    const previous = document.activeElement as HTMLElement | null;
    const rect = node.getBoundingClientRect();
    node.style.left = `${Math.max(4, Math.min(x, window.innerWidth - rect.width - 4))}px`;
    node.style.top = `${Math.max(4, Math.min(y, window.innerHeight - rect.height - 4))}px`;
    node.querySelector<HTMLElement>('[role="menuitem"]')?.focus();
    const outside = (e: PointerEvent) => { if (!node.contains(e.target as Node)) onClose(); };
    const close = () => onClose();
    document.addEventListener("pointerdown", outside);
    window.addEventListener("scroll", close, true);
    window.addEventListener("resize", close);
    return () => {
      document.removeEventListener("pointerdown", outside);
      window.removeEventListener("scroll", close, true);
      window.removeEventListener("resize", close);
      if (node.contains(document.activeElement)) previous?.focus();
    };
  }, [x, y, onClose, entries]);
  return createPortal(<div ref={ref} role="menu" aria-label={submenu ? "Mover a estado" : "Acciones del item"}
    style={{ left: x, top: y }} className="fixed z-[100] max-h-[calc(100vh-8px)] w-64 max-w-[calc(100vw-8px)] overflow-auto rounded-md border border-slate-200 bg-white p-1 text-sm shadow-lg dark:border-border dark:bg-surface"
    onContextMenu={(e) => e.preventDefault()} onKeyDown={(e) => {
      const nodes = Array.from(ref.current!.querySelectorAll<HTMLElement>('[role="menuitem"]'));
      const index = nodes.indexOf(document.activeElement as HTMLElement);
      if (["ArrowDown", "ArrowUp", "Home", "End"].includes(e.key)) {
        e.preventDefault();
        nodes[e.key === "Home" ? 0 : e.key === "End" ? nodes.length - 1 : (index + (e.key === "ArrowDown" ? 1 : -1) + nodes.length) % nodes.length]?.focus();
      } else if (e.key === "Escape" || e.key === "Tab") { e.preventDefault(); onClose(); }
      else if (e.key === "ArrowLeft" && submenu) { e.preventDefault(); setSubmenu(null); }
      else if (e.key === "ArrowRight" && entries[index - (submenu ? 1 : 0)]?.children) { e.preventDefault(); setSubmenu(entries[index - (submenu ? 1 : 0)].children!); }
    }}>
    {submenu && <button role="menuitem" className="w-full rounded px-3 py-2 text-left hover:bg-slate-100 focus:bg-slate-100 dark:hover:bg-raised dark:focus:bg-raised" onClick={() => setSubmenu(null)}><ArrowLeft aria-hidden="true" className="mr-1 inline size-4" />Volver</button>}
    {entries.map((action) => <div key={action.label}>
      {action.content ? <div onClick={onClose}>{action.content}</div> : <button role="menuitem" aria-haspopup={action.children ? "menu" : undefined} aria-expanded={action.children ? false : undefined}
        className="w-full rounded px-3 py-2 text-left hover:bg-slate-100 focus:bg-slate-100 dark:hover:bg-raised dark:focus:bg-raised"
        onClick={() => { if (action.children) setSubmenu(action.children); else { onClose(); action.run?.(); } }}>{action.label}{action.children ? " ›" : ""}</button>}
    </div>)}
  </div>, document.body);
}
