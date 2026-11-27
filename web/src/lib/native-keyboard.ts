// The Android app lets the keyboard overlay the page and calls
// __doryKeyboard with the height it covers (CSS px) on every frame of its
// animation. Only the screen frames (.keyboard-follow) change height, inline
// and in px: the top stays put, the bottom rises with the keyboard, and no
// shared CSS variable makes the whole document recalculate its styles.
declare global {
  interface Window { __doryKeyboard?: (px: number) => void }
}

let current = 0;

function apply(px: number) {
  const room = (extra: string) => (px > 0 ? `max(var(--tabbar-base), ${px}px)${extra}` : "");
  for (const el of document.querySelectorAll<HTMLElement>(".keyboard-follow")) {
    el.style.height = px > 0 ? `calc(100dvh - 4rem - 1px - ${room("")})` : "";
  }
  for (const el of document.querySelectorAll<HTMLElement>("[data-keyboard=pad]")) {
    el.style.paddingBottom = px > 0 ? `calc(2rem + ${px}px)` : "";
  }
}

window.__doryKeyboard = (px) => {
  if (px === current) return;
  current = px;
  apply(px);
};

// Screens mounted while the keyboard is up take its height too.
new MutationObserver(() => { if (current > 0) apply(current); }).observe(document.body, { childList: true, subtree: true });

export {};
