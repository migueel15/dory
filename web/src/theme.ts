import { useEffect, useState } from "react";

export type Theme = "light" | "dark" | "system";
const KEY = "theme";
const media = window.matchMedia("(prefers-color-scheme: dark)");

const stored = (): Theme => {
  const t = localStorage.getItem(KEY);
  return t === "light" || t === "dark" ? t : "system";
};

const isDark = (t: Theme) => t === "dark" || (t === "system" && media.matches);

export function applyTheme(t: Theme = stored()) {
  document.documentElement.classList.toggle("dark", isDark(t));
}

// Theme choice persisted in localStorage; "system" tracks the OS preference live.
export function useTheme() {
  const [theme, setTheme] = useState<Theme>(stored);
  const [dark, setDark] = useState(() => isDark(theme));
  useEffect(() => {
    if (theme === "system") localStorage.removeItem(KEY);
    else localStorage.setItem(KEY, theme);
    const apply = () => {
      applyTheme(theme);
      setDark(isDark(theme));
    };
    apply();
    if (theme !== "system") return;
    const onChange = apply;
    media.addEventListener("change", onChange);
    return () => media.removeEventListener("change", onChange);
  }, [theme]);
  return [dark, setTheme] as const;
}
