import { useEffect, useState } from "react";
import { applyScale } from "../scale";
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

export const DEFAULT_SIZE = { ui: 16, terminal: 13 } as const;
export const SIZE_RANGE = { ui: [12, 24], terminal: [8, 32] } as const;
const sizeKey = (k: Kind) => (k === "ui" ? storageKeys.uiFontSize : storageKeys.terminalFontSize);

// fontSize is the size in px chosen for the interface or the terminal, or the default.
export function fontSize(k: Kind): number {
  const n = Number(storage.get(sizeKey(k)));
  const [min, max] = SIZE_RANGE[k];
  return n >= min && n <= max ? n : DEFAULT_SIZE[k];
}

export function chooseSize(k: Kind, size: number) {
  const [min, max] = SIZE_RANGE[k];
  const n = Math.round(Math.min(max, Math.max(min, size)));
  if (n === DEFAULT_SIZE[k]) storage.remove(sizeKey(k)); else storage.set(sizeKey(k), String(n));
  if (k === "ui") applyScale();
  events.fontsChanged.emit();
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

// defaultFontName is the family the default stack really draws with on this device.
export function defaultFontName(k: Kind): string {
  const has = detector();
  const stack = k === "ui" ? DEFAULT_UI_FONT : DEFAULT_TERMINAL_FONT;
  for (const raw of stack.split(",")) {
    const family = raw.trim().replace(/^"|"$/g, "");
    if (/^(system-ui|ui-sans-serif|ui-monospace|sans-serif|monospace|serif)$/.test(family)) continue;
    if (has(family)) return family;
  }
  return k === "ui" ? "Sans-serif" : "Monospace";
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
