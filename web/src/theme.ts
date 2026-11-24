import { useEffect, useState } from "react";
import { events } from "./lib/store";

export type Theme = "light" | "dark" | "system";
const KEY = "theme";
const media = window.matchMedia("(prefers-color-scheme: dark)");

export const storedTheme = (): Theme => {
  const t = localStorage.getItem(KEY);
  return t === "light" || t === "dark" ? t : "system";
};

const isDark = (t: Theme) => t === "dark" || (t === "system" && media.matches);

export function applyTheme(t: Theme = storedTheme()) {
  document.documentElement.classList.toggle("dark", isDark(t));
}

// setTheme saves the choice of this browser and tells every useTheme.
export function setTheme(t: Theme) {
  if (t === "system") localStorage.removeItem(KEY);
  else localStorage.setItem(KEY, t);
  applyTheme(t);
  events.themeChanged.emit();
}

// Theme choice persisted in localStorage; "system" tracks the OS preference live.
// Every caller shares it: the header toggle and the settings stay in sync.
export function useTheme() {
  const [theme, setLocal] = useState<Theme>(storedTheme);
  const [dark, setDark] = useState(() => isDark(theme));
  useEffect(() => {
    const sync = () => {
      const t = storedTheme();
      setLocal(t);
      setDark(isDark(t));
      applyTheme(t);
    };
    const off = events.themeChanged.on(sync);
    media.addEventListener("change", sync);
    return () => { off(); media.removeEventListener("change", sync); };
  }, []);
  return [dark, setTheme, theme] as const;
}
