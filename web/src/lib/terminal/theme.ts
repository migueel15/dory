import type { ITheme } from "@xterm/xterm";

// The colors of the terminal follow the app's theme (the `dark` class on <html>).
const css = (name: string, fallback: string) => getComputedStyle(document.documentElement).getPropertyValue(name).trim() || fallback;

export const isDark = () => document.documentElement.classList.contains("dark");

export function terminalTheme(): ITheme {
  if (isDark()) {
    return {
      background: css("--color-canvas", "#0f0f11"), foreground: css("--color-text", "#e6e6e8"),
      cursor: css("--color-accent-text", "#a5acf5"), cursorAccent: css("--color-canvas", "#0f0f11"),
      selectionBackground: "rgba(180,180,189,0.30)",
      black: "#2b2b31", red: "#f0707a", green: "#7bd88f", yellow: "#e5c07b", blue: "#7aa2f7", magenta: "#bb9af7", cyan: "#56d4dd", white: "#d4d4da",
      brightBlack: "#6b6b76", brightRed: "#ff8a93", brightGreen: "#98e6a8", brightYellow: "#f0d397", brightBlue: "#9bb8ff", brightMagenta: "#cfb4ff", brightCyan: "#7ae3ea", brightWhite: "#f4f4f6",
    };
  }
  return {
    background: "#ffffff", foreground: "#0f172a", cursor: "#4f46e5", cursorAccent: "#ffffff",
    selectionBackground: "rgba(79,70,229,0.22)",
    black: "#1e293b", red: "#c92a3a", green: "#2f8f4a", yellow: "#9a6b00", blue: "#2563c9", magenta: "#8a3fc4", cyan: "#0e7f8c", white: "#cbd5e1",
    brightBlack: "#64748b", brightRed: "#e5484d", brightGreen: "#3da35a", brightYellow: "#b7791f", brightBlue: "#3b82f6", brightMagenta: "#a855f7", brightCyan: "#1597a6", brightWhite: "#f1f5f9",
  };
}

export const terminalFont = () => css("--font-mono", "") || 'ui-monospace, "SF Mono", Menlo, Consolas, "DejaVu Sans Mono", monospace';
