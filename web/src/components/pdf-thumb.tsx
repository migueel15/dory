import { useEffect, useRef, useState } from "react";
import { FileText } from "@phosphor-icons/react";

// pdf.js loads only when a PDF thumbnail is shown, so it stays out of the main bundle.
const pdfjs = () => Promise.all([import("pdfjs-dist"), import("pdfjs-dist/build/pdf.worker.min.mjs?url")]).then(([lib, worker]) => {
  lib.GlobalWorkerOptions.workerSrc = worker.default;
  return lib;
});

// PdfThumb draws the first page of a PDF; while loading or on error it shows a document icon.
export function PdfThumb({ src, width, className }: { src: string; width: number; className?: string }) {
  const canvas = useRef<HTMLCanvasElement>(null);
  const [ok, setOk] = useState(false);
  useEffect(() => {
    let cancelled = false;
    let destroy: (() => void) | undefined;
    pdfjs().then(async (lib) => {
      const task = lib.getDocument({ url: src, withCredentials: true });
      destroy = () => void task.destroy();
      const doc = await task.promise;
      const page = await doc.getPage(1);
      const base = page.getViewport({ scale: 1 });
      const scale = (width * (window.devicePixelRatio || 1)) / base.width;
      const vp = page.getViewport({ scale });
      const c = canvas.current;
      if (cancelled || !c) return;
      c.width = vp.width;
      c.height = vp.height;
      await page.render({ canvas: c, canvasContext: c.getContext("2d")!, viewport: vp }).promise;
      if (!cancelled) setOk(true);
    }).catch(() => {});
    return () => { cancelled = true; destroy?.(); };
  }, [src, width]);
  return (
    <span className={`relative block overflow-hidden bg-white ${className ?? ""}`}>
      <canvas ref={canvas} style={{ width }} className={`block ${ok ? "" : "hidden"}`} />
      {!ok && <span className="grid size-full place-items-center text-slate-400"><FileText className="size-6" /></span>}
    </span>
  );
}
