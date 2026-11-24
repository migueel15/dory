import { useEffect, useState } from "react";
import { events } from "./store";
import { storage, storageKeys } from "./storage";

// Fonts of the interface and of the terminal, chosen per device among the
// ones installed. The choice goes first in a stack that keeps the defaults
// behind it: a font that is missing, or lacks a glyph, falls back to them and
// finally to the generic family of the system.
export const DEFAULT_UI_FONT = '"Adwaita Sans", Inter, system-ui, ui-sans-serif, sans-serif';
// Nerd Fonts go first in the terminal so the icons of prompts and tools draw.
const NERD = ['"CaskaydiaMono Nerd Font"', '"CaskaydiaCove Nerd Font Mono"', '"JetBrainsMono Nerd Font"', '"FiraCode Nerd Font"', '"Hack Nerd Font"', '"MesloLGS NF"', '"Iosevka Nerd Font"', '"Symbols Nerd Font Mono"', '"Symbols Nerd Font"'];
export const DEFAULT_TERMINAL_FONT = [...NERD, 'ui-monospace', '"SF Mono"', 'Menlo', 'Consolas', '"DejaVu Sans Mono"', 'monospace'].join(", ");
// Symbols that a plain font lacks still come from a Nerd symbols font when one is installed.
const SYMBOLS = '"Symbols Nerd Font Mono", "Symbols Nerd Font"';

type Kind = "ui" | "terminal";
const key = (k: Kind) => (k === "ui" ? storageKeys.uiFont : storageKeys.terminalFont);
const quote = (f: string) => `"${f.replace(/["\\]/g, "")}"`;

export const chosenFont = (k: Kind) => storage.get(key(k)) ?? "";

export function terminalFontStack(): string {
  const f = chosenFont("terminal");
  return f ? `${quote(f)}, ${SYMBOLS}, ${DEFAULT_TERMINAL_FONT}` : DEFAULT_TERMINAL_FONT;
}

// applyUiFont sets the font of the interface and of the answers.
export function applyUiFont() {
  const f = chosenFont("ui");
  const stack = f ? `${quote(f)}, ${DEFAULT_UI_FONT}` : DEFAULT_UI_FONT;
  const root = document.documentElement.style;
  root.setProperty("--font-sans", stack);
  root.setProperty("--font-reading", stack);
}

export function chooseFont(k: Kind, family: string) {
  if (family) storage.set(key(k), family); else storage.remove(key(k));
  if (k === "ui") applyUiFont();
  events.fontsChanged.emit();
}

// Families to try when the system cannot list its fonts (a browser without the
// Local Font Access API): each is kept only if it is really installed.
const KNOWN_SANS = ["Inter", "Roboto", "Open Sans", "Noto Sans", "Ubuntu", "Cantarell", "Adwaita Sans", "Segoe UI", "SF Pro Text", "Helvetica Neue", "Arial", "Lato", "Source Sans 3", "IBM Plex Sans", "Fira Sans"];
const KNOWN_MONO = ["CaskaydiaMono Nerd Font", "CaskaydiaCove Nerd Font Mono", "JetBrainsMono Nerd Font", "FiraCode Nerd Font", "Hack Nerd Font", "MesloLGS NF", "Iosevka Nerd Font", "JetBrains Mono", "Fira Code", "Cascadia Code", "Source Code Pro", "IBM Plex Mono", "DejaVu Sans Mono", "Ubuntu Mono", "Adwaita Mono", "Menlo", "Consolas", "SF Mono", "Courier New"];

// has tells whether a family is installed: text in it measures differently from the generic fallback.
function detector() {
  const ctx = document.createElement("canvas").getContext("2d")!;
  const sample = "mmmmmmmmmmlliWi10.@#あ";
  const width = (font: string) => { ctx.font = `32px ${font}`; return ctx.measureText(sample).width; };
  const bases = ["monospace", "serif", "sans-serif"].map((b) => [b, width(b)] as const);
  return (family: string) => bases.some(([b, w]) => width(`${quote(family)}, ${b}`) !== w);
}

// isMono tells whether every glyph of an installed family has the same width.
function monoCheck() {
  const ctx = document.createElement("canvas").getContext("2d")!;
  return (family: string) => {
    ctx.font = `32px ${quote(family)}, serif`;
    const w = ctx.measureText("i").width;
    return w > 0 && Math.abs(w - ctx.measureText("W").width) < 0.01 && Math.abs(w - ctx.measureText("m").width) < 0.01;
  };
}

type LocalFont = { family: string };
let cache: Promise<string[] | null> | null = null;
const listAll = () => cache ??= (async () => {
  const query = (window as unknown as { queryLocalFonts?: () => Promise<LocalFont[]> }).queryLocalFonts;
  if (!query) return null;
  try { return [...new Set((await query.call(window)).map((f) => f.family))].sort((a, b) => a.localeCompare(b)); } catch { return null; }
})();

// installedFonts lists the families of this device, or only the monospaced ones.
export async function installedFonts(mono: boolean): Promise<string[]> {
  const all = await listAll();
  if (all) {
    if (!mono) return all;
    const isMono = monoCheck();
    return all.filter(isMono);
  }
  const has = detector();
  const found = (mono ? KNOWN_MONO : KNOWN_SANS).filter(has);
  return mono ? found.filter(monoCheck()) : found;
}

// useInstalledFonts is the list for the pickers: null while it loads.
export function useInstalledFonts(mono: boolean): string[] | null {
  const [fonts, set] = useState<string[] | null>(null);
  useEffect(() => { let live = true; void installedFonts(mono).then((f) => live && set(f)); return () => { live = false; }; }, [mono]);
  return fonts;
}
