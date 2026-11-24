import { useEffect, useState } from "react";
import { Desktop, Moon, Sun } from "@phosphor-icons/react";
import { MAX_SCALE, MIN_SCALE, setScale, useScale } from "../../scale";
import { useTheme, type Theme } from "../../theme";

const THEMES: { value: Theme; label: string; icon: typeof Sun }[] = [
  { value: "light", label: "Claro", icon: Sun },
  { value: "dark", label: "Oscuro", icon: Moon },
  { value: "system", label: "Sistema", icon: Desktop },
];

// AppearanceSettings changes the theme and the scale of the interface on this device.
export function AppearanceSettings() {
  const [, setTheme, theme] = useTheme();
  const scale = useScale();
  // The slider moves a draft; the interface rescales only when it is released.
  const [draft, setDraft] = useState(Math.round(scale * 100));
  useEffect(() => setDraft(Math.round(scale * 100)), [scale]);
  const commit = () => draft !== Math.round(scale * 100) && setScale(draft / 100);
  const min = MIN_SCALE * 100, max = MAX_SCALE * 100;
  const fill = ((draft - min) / (max - min)) * 100;
  return (
    <div className="space-y-6 p-4">
      <div className="space-y-2">
        <span className="block text-sm font-medium">Tema</span>
        <div role="radiogroup" aria-label="Tema" className="inline-flex rounded-lg border border-slate-200 p-0.5 dark:border-border">
          {THEMES.map(({ value, label, icon: Icon }) => (
            <button key={value} type="button" role="radio" aria-checked={theme === value} onClick={() => setTheme(value)}
              className={`flex items-center gap-1.5 rounded-md px-3 py-1.5 text-sm transition-colors ${theme === value ? "bg-slate-900 text-white dark:bg-accent dark:text-white" : "text-slate-600 hover:bg-slate-100 dark:text-muted dark:hover:bg-hover dark:hover:text-white"}`}>
              <Icon className="size-4" aria-hidden="true" />{label}
            </button>
          ))}
        </div>
        <p className="text-xs text-slate-500 dark:text-muted">Sistema sigue la preferencia del sistema operativo. Se guarda en este navegador.</p>
      </div>
      <div className="max-w-sm space-y-2">
        <label htmlFor="ui-scale" className="flex items-center text-sm font-medium">
          Escala de la interfaz
          <span className="ml-auto rounded bg-slate-100 px-1.5 py-0.5 font-mono text-xs text-slate-600 dark:bg-raised dark:text-muted">{draft}%</span>
        </label>
        <input id="ui-scale" type="range" min={min} max={max} step={1} value={draft} className="ui-range w-full"
          style={{ "--fill": `${fill}%` } as React.CSSProperties}
          onChange={(e) => setDraft(Number(e.target.value))} onPointerUp={commit} onKeyUp={commit} onBlur={commit} />
        <div className="flex justify-between text-[11px] text-slate-400 dark:text-muted">
          <span>{min}%</span>
          <button type="button" className="hover:text-slate-700 dark:hover:text-white" onClick={() => setScale(1)}>100%</button>
          <span>{max}%</span>
        </div>
        <p className="text-xs text-slate-500 dark:text-muted">
          Se aplica al soltar. También con Ctrl + / Ctrl − (de 5 en 5) y Ctrl 0 para volver al 100% (Cmd en macOS). Se guarda en este navegador.
        </p>
      </div>
    </div>
  );
}
