import { useEffect, useState, type ComponentType } from "react";
import { Cpu, Microphone, X, type IconProps } from "@phosphor-icons/react";
import { ProvidersSettings } from "./settings-providers";
import { DictationSettings } from "./settings-dictation";

type Section = { id: string; label: string; icon: ComponentType<IconProps>; component: ComponentType };

// SECTIONS are the parts of the settings, in the order of the side bar.
const SECTIONS: Section[] = [
  { id: "providers", label: "Proveedores", icon: Cpu, component: ProvidersSettings },
  { id: "dictation", label: "Dictado", icon: Microphone, component: DictationSettings },
];

// SettingsModal covers most of the screen over a dimmed background. A click
// outside it or Escape closes it.
export function SettingsModal({ onClose }: { onClose: () => void }) {
  const [active, setActive] = useState(SECTIONS[0].id);
  useEffect(() => {
    const onKey = (e: KeyboardEvent) => { if (e.key === "Escape") onClose(); };
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [onClose]);
  const section = SECTIONS.find((s) => s.id === active) ?? SECTIONS[0];
  const Body = section.component;
  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/60 p-2 sm:p-6" onClick={onClose}>
      <div role="dialog" aria-modal="true" aria-label="Ajustes" onClick={(e) => e.stopPropagation()}
        className="flex h-full max-h-[56rem] w-full max-w-6xl flex-col overflow-hidden rounded-xl border border-slate-200 bg-white shadow-2xl dark:border-border dark:bg-surface md:flex-row">
        <nav aria-label="Secciones de ajustes" className="flex shrink-0 gap-1 overflow-x-auto border-b border-slate-200 p-2 dark:border-border md:w-60 md:flex-col md:border-b-0 md:border-r md:p-3">
          {SECTIONS.map(({ id, label, icon: Icon }) => (
            <button key={id} type="button" onClick={() => setActive(id)} aria-current={id === active ? "page" : undefined}
              className={`flex items-center gap-2 whitespace-nowrap rounded-md px-3 py-2 text-sm ${id === active ? "bg-slate-100 font-semibold dark:bg-raised" : "text-slate-600 hover:bg-slate-50 dark:text-muted dark:hover:bg-raised"}`}>
              <Icon className="size-4" aria-hidden="true" />{label}
            </button>
          ))}
        </nav>
        <div className="flex min-h-0 min-w-0 flex-1 flex-col">
          <header className="flex items-center gap-2 border-b border-slate-200 px-5 py-3 text-sm dark:border-border">
            <span className="text-slate-500 dark:text-muted">Ajustes</span>
            <span className="text-slate-400">/</span>
            <span className="font-semibold">{section.label}</span>
            <button type="button" aria-label="Cerrar" onClick={onClose} className="ml-auto grid size-8 place-items-center rounded text-slate-500 hover:bg-slate-100 dark:text-muted dark:hover:bg-raised">
              <X className="size-4" aria-hidden="true" />
            </button>
          </header>
          <div className="min-h-0 flex-1 overflow-auto p-5 sm:p-8">
            <div className="mx-auto max-w-4xl"><Body /></div>
          </div>
        </div>
      </div>
    </div>
  );
}
