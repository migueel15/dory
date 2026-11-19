import { useEffect, useState } from "react";

// Interface scale, stored per browser and applied as CSS zoom on the root so
// it behaves the same in the browser and the desktop app.
const KEY = "ui-scale";
const EVENT = "ui-scale";
export const SCALES = [0.75, 0.8, 0.9, 1, 1.1, 1.25, 1.5, 1.75, 2];

const stored = () => {
  const s = Number(localStorage.getItem(KEY));
  return SCALES.includes(s) ? s : 1;
};

export function setScale(s: number) {
  if (s === 1) localStorage.removeItem(KEY);
  else localStorage.setItem(KEY, String(s));
  applyScale(s);
  window.dispatchEvent(new Event(EVENT));
}

export function applyScale(s = stored()) {
  document.documentElement.style.zoom = s === 1 ? "" : String(s);
}

// step moves one scale up or down, or back to 100% with 0.
function step(dir: -1 | 0 | 1) {
  if (dir === 0) return setScale(1);
  const i = SCALES.indexOf(stored());
  setScale(SCALES[Math.max(0, Math.min(SCALES.length - 1, i + dir))]);
}

// listenZoomKeys binds Ctrl (Cmd on macOS) with +, - and 0 to the scale,
// instead of the browser zoom.
export function listenZoomKeys() {
  window.addEventListener("keydown", (e) => {
    if (!(e.ctrlKey || e.metaKey) || e.altKey) return;
    const dir = e.key === "+" || e.key === "=" ? 1 : e.key === "-" || e.key === "_" ? -1 : e.key === "0" ? 0 : null;
    if (dir === null) return;
    e.preventDefault();
    step(dir);
  });
}

export function useScale() {
  const [scale, set] = useState(stored);
  useEffect(() => {
    const sync = () => set(stored());
    window.addEventListener(EVENT, sync);
    return () => window.removeEventListener(EVENT, sync);
  }, []);
  return scale;
}
