import { useRef, useState } from "react";
import { FileText, Paperclip, X } from "@phosphor-icons/react";
import type { Attachment } from "../api/client";
import { errMsg, iconButton, uuid } from "./ui";

export type DraftFile = { key: string; file: File; name: string; mime: string; preview: string; done?: Attachment; error?: string };

// useComposerFiles holds the files of a message box: pasted, dropped or
// picked. With upload, each file goes up as soon as it is added (a thread
// that exists); without it they wait for the box to send them.
export function useComposerFiles(upload?: (file: File) => Promise<Attachment>) {
  const [files, setFiles] = useState<DraftFile[]>([]);
  const picker = useRef<HTMLInputElement>(null);
  const add = (list: FileList | File[]) => {
    for (const file of Array.from(list)) {
      const key = uuid();
      const named = file.name ? file : new File([file], `imagen-${new Date().toISOString().slice(11, 19).replace(/:/g, "")}.png`, { type: file.type });
      setFiles((fs) => [...fs, { key, file: named, name: named.name, mime: named.type, preview: URL.createObjectURL(named) }]);
      upload?.(named).then(
        (done) => setFiles((fs) => fs.map((f) => (f.key === key ? { ...f, done } : f))),
        (e: unknown) => setFiles((fs) => fs.map((f) => (f.key === key ? { ...f, error: errMsg(e) || "No se pudo subir" } : f))),
      );
    }
  };
  const remove = (key: string) => setFiles((fs) => fs.filter((f) => { if (f.key === key) URL.revokeObjectURL(f.preview); return f.key !== key; }));
  const clear = () => setFiles((fs) => { fs.forEach((f) => URL.revokeObjectURL(f.preview)); return []; });
  const uploading = !!upload && files.some((f) => !f.done && !f.error);
  // Props for the box's form: drop and paste add files.
  const formProps = {
    onDragOver: (e: React.DragEvent) => { if (e.dataTransfer.types.includes("Files")) e.preventDefault(); },
    onDrop: (e: React.DragEvent) => { if (e.dataTransfer.files.length) { e.preventDefault(); add(e.dataTransfer.files); } },
    onPaste: (e: React.ClipboardEvent) => {
      // Screenshots often come only as clipboard items, not in .files.
      const fs = Array.from(e.clipboardData.items).flatMap((i) => { const f = i.kind === "file" ? i.getAsFile() : null; return f ? [f] : []; });
      if (fs.length) { e.preventDefault(); add(fs); }
    },
  };
  const tray = files.length > 0 && (
    <div className="flex flex-wrap gap-2 px-3 pt-3">
      {files.map((f) => (
        <div key={f.key} title={f.error ?? f.name} className={`group relative size-14 shrink-0 overflow-hidden rounded-xl border bg-slate-50 dark:bg-raised ${f.error ? "border-red-400" : "border-slate-200 dark:border-border"}`}>
          <a href={f.preview} target="_blank" rel="noreferrer" aria-label={`Ver ${f.name}`} className="block size-14 cursor-zoom-in">
            {f.mime.startsWith("image/") ? <img src={f.preview} alt="" className="size-14 shrink-0 object-cover" /> : <span className="grid size-14 shrink-0 place-items-center text-slate-400"><FileText className="size-6" /></span>}
          </a>
          {f.error && <span className="absolute inset-x-0 bottom-0 bg-red-500 text-center text-[0.5625rem] font-medium text-white">Error</span>}
          {upload && !f.done && !f.error && <span className="absolute inset-0 grid place-items-center bg-white/60 dark:bg-black/40"><span className="size-4 animate-spin rounded-full border-2 border-current border-t-transparent" /></span>}
          <button type="button" onClick={() => remove(f.key)} aria-label={`Quitar ${f.name}`} className="absolute right-1 top-1 grid size-5 place-items-center rounded-full bg-slate-900/70 text-white transition-opacity hover:bg-slate-900 sm:opacity-0 sm:group-hover:opacity-100 sm:focus-visible:opacity-100"><X weight="regular" className="size-3" /></button>
        </div>
      ))}
    </div>
  );
  const attachButton = <>
    <input ref={picker} type="file" multiple hidden onChange={(e) => { if (e.target.files) add(e.target.files); e.target.value = ""; }} />
    <button type="button" onClick={() => picker.current?.click()} title="Adjuntar imágenes o ficheros" aria-label="Adjuntar" className={iconButton}>
      <Paperclip weight="regular" className="size-4" />
    </button>
  </>;
  return { files, add, remove, clear, uploading, formProps, tray, attachButton };
}
