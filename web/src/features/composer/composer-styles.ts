// Shared classes of the message boxes (start screen, threads and phones).
export const composerFrame = "relative z-10 rounded-3xl border border-slate-200 bg-white shadow-md dark:border-raised dark:bg-surface";
export const composerArea = "relative block max-h-60 w-full resize-none break-words bg-transparent px-4 pb-1 pt-3.5 text-[0.9375rem] leading-6 !outline-none placeholder:text-slate-400";

// autosizeTextarea resizes a textarea to its content without momentarily
// collapsing the layout around it: locking the parent's min-height during the
// temporary height="auto" probe prevents flex siblings (such as the chat
// scroller above the composer) from growing and clamping scrollTop, which
// otherwise makes messages jump up and snap back on every keystroke once the
// draft spans two or more lines.
export function autosizeTextarea(el: HTMLTextAreaElement, maxHeight = Infinity) {
  const parent = el.parentElement;
  const prevMin = parent?.style.minHeight ?? "";
  const prevScroll = el.scrollTop;
  const locked = parent ? parent.offsetHeight : 0;
  if (parent && locked > 0) parent.style.minHeight = `${locked}px`;
  el.style.height = "auto";
  const next = Math.min(el.scrollHeight, maxHeight);
  el.style.height = `${next}px`;
  if (parent) parent.style.minHeight = prevMin;
  if (prevScroll > 0) el.scrollTop = prevScroll;
}
