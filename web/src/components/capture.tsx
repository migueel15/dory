import { useEffect, useRef, useState } from "react";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { api } from "../api/client";
import { ErrorText } from "./ui";
import { useProject } from "../project";

// QuickCapture saves an idea from the home page; "/" focuses it unless the user is typing.
export function QuickCapture() {
  const qc = useQueryClient();
  const project = useProject();
  const [text, setText] = useState("");
  const input = useRef<HTMLInputElement>(null);
  const save = useMutation({
    mutationFn: (raw: string) => api.items.create({ title: raw.split("\n")[0].slice(0, 80), raw_text: raw, status: "inbox", tags: [], project_id: project || null }),
    onSuccess: () => { setText(""); qc.invalidateQueries({ queryKey: ["items"] }); },
  });
  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      const typing = e.target instanceof HTMLElement && e.target.closest("input, textarea, select, [contenteditable]");
      if (e.key === "/" && !typing) { e.preventDefault(); input.current?.focus(); }
    };
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, []);
  return (
    <form onSubmit={(e) => { e.preventDefault(); if (text.trim()) save.mutate(text.trim()); }}>
      <input ref={input} value={text} onChange={(e) => setText(e.target.value)} disabled={save.isPending} aria-label="Nueva idea" placeholder="Nueva idea… (/ · Ctrl+K para comandos)" className="w-full rounded border border-slate-300 bg-white px-3 py-2 text-sm dark:border-slate-700 dark:bg-slate-900" />
      <ErrorText error={save.error} />
    </form>
  );
}
