import { useEffect, useState } from "react";
import { Desktop, Moon, Sun } from "@phosphor-icons/react";
import { MAX_SCALE, MIN_SCALE, setScale, useScale } from "../../scale";
import { Select } from "../../components/ui";
import { DEFAULT_TERMINAL_FONT, DEFAULT_UI_FONT, chosenFont, chooseFont, defaultFontName, chooseSize, fontSize, DEFAULT_SIZE, SIZE_RANGE, useInstalledFonts } from "../../lib/fonts";
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
  const ui = useInstalledFonts(false);
  const mono = useInstalledFonts(true);
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
      <FontPicker id="ui-font" label="Fuente de la interfaz" kind="ui" fonts={ui} fallback={DEFAULT_UI_FONT} />
      <FontPicker id="terminal-font" label="Fuente de la terminal" kind="terminal" fonts={mono} fallback={DEFAULT_TERMINAL_FONT} />
    </div>
  );
}

// FontPicker chooses a font among the ones installed on this device. The
// choice goes first in the stack and the defaults stay behind it, so a font
// that is missing or lacks a glyph falls back instead of breaking the text.
function FontPicker({ id, label, kind, fonts, fallback }: { id: string; label: string; kind: "ui" | "terminal"; fonts: string[] | null; fallback: string }) {
  const [, bump] = useState(0);
  // The picker always shows the family in use: the chosen one or, with no choice, the one the defaults draw with.
  const [fallbackName] = useState(() => defaultFontName(kind));
  const chosen = chosenFont(kind);
  const value = chosen || fallbackName;
  const size = fontSize(kind);
  const names = [...new Set([value, ...(fonts ?? [])])].sort((a, b) => a.localeCompare(b));
  const options = names.map((f) => ({ value: f, label: f === chosen && !fonts?.includes(f) ? `${f} (no instalada)` : f }));
  return (
    <div className="max-w-md space-y-2">
      <span id={id} className="block text-sm font-medium">{label}</span>
      <div className="flex items-center gap-2">
        <div className="min-w-0 flex-1"><Select value={value} options={options} placeholder={value} onChange={(v) => { chooseFont(kind, v === fallbackName ? "" : v); bump((n) => n + 1); }} /></div>
        <input type="number" aria-label={`Tamaño de ${label.toLowerCase()}`} title="Tamaño en px" min={SIZE_RANGE[kind][0]} max={SIZE_RANGE[kind][1]} step={1} value={size} placeholder={String(DEFAULT_SIZE[kind])}
          onChange={(e) => { if (e.target.value !== "") { chooseSize(kind, Number(e.target.value)); bump((n) => n + 1); } }}
          className="w-16 rounded-md border border-slate-300 bg-white px-2 py-1.5 text-sm dark:border-border-strong dark:bg-surface" />
        <span className="text-xs text-slate-500 dark:text-muted">px</span>
        <button type="button" disabled={size === DEFAULT_SIZE[kind]} onClick={() => { chooseSize(kind, DEFAULT_SIZE[kind]); bump((n) => n + 1); }} className="text-xs text-slate-500 hover:text-slate-900 disabled:invisible dark:text-muted dark:hover:text-white">Restablecer</button>
      </div>
      <p className="text-xs text-slate-500 dark:text-muted">
        {fonts && fonts.length === 0 ? "No se han podido listar las fuentes de este equipo; se usa la de siempre. " : ""}
        Solo se ofrecen las fuentes instaladas en este equipo{kind === "terminal" ? " de ancho fijo. Para ver los iconos de la terminal elige una Nerd Font" : ""}. Si falta o no tiene un símbolo, se usa la siguiente: {fallback.replace(/"/g, "").split(",").slice(0, 3).join(", ")}… Se guarda en este navegador.
      </p>
    </div>
  );
}
