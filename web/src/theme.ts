import { useEffect, useState } from "react";

export type Theme = "light" | "dark" | "system";
const KEY = "theme";
const media = window.matchMedia("(prefers-color-scheme: dark)");

const stored = (): Theme => {
  const t = localStorage.getItem(KEY);
  return t === "light" || t === "dark" ? t : "system";
};

export function applyTheme(t: Theme = stored()) {
  document.documentElement.classList.toggle("dark", t === "dark" || (t === "system" && media.matches));
}

// Theme choice persisted in localStorage; "system" tracks the OS preference live.
export function useTheme() {
  const [theme, setTheme] = useState<Theme>(stored);
  useEffect(() => {
    if (theme === "system") localStorage.removeItem(KEY);
    else localStorage.setItem(KEY, theme);
    applyTheme(theme);
    if (theme !== "system") return;
    const onChange = () => applyTheme("system");
    media.addEventListener("change", onChange);
    return () => media.removeEventListener("change", onChange);
  }, [theme]);
  return [theme, setTheme] as const;
}
