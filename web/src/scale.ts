import { useEffect, useState } from "react";

// Interface scale, stored per browser and applied as the root font size, so
// every rem-based size grows or shrinks while the layout still fills the
// window (CSS zoom would break vh, dvh and window measurements).
const KEY = "ui-scale";
const EVENT = "ui-scale";
export const MIN_SCALE = 0.5;
export const MAX_SCALE = 2;
const clamp = (s: number) => Math.round(Math.min(MAX_SCALE, Math.max(MIN_SCALE, s)) * 100) / 100;

const stored = () => {
  const s = Number(localStorage.getItem(KEY));
  return s >= MIN_SCALE && s <= MAX_SCALE ? s : 1;
};

export function setScale(s: number) {
  s = clamp(s);
  if (s === 1) localStorage.removeItem(KEY);
  else localStorage.setItem(KEY, String(s));
  applyScale(s);
  window.dispatchEvent(new Event(EVENT));
}

export function applyScale(s = stored()) {
  // The desktop app zooms the page itself: breakpoints, px sizes and window
  // measurements follow the scale, like the browser zoom.
  const zoom = window.doryDesktop?.setZoom;
  if (zoom) return zoom(s);
  document.documentElement.style.fontSize = s === 1 ? "" : `${s * 100}%`;
}

// step moves the scale 5% up or down, or back to 100% with 0.
function step(dir: -1 | 0 | 1) {
  if (dir === 0) return setScale(1);
  setScale(stored() + dir * 0.05);
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
