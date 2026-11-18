import { useState } from "react";
import { chordFromEvent, formatChord, saveDictationShortcut, useDictationShortcutSetting } from "../components/dictation-shortcut";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { Check, DownloadSimple } from "@phosphor-icons/react";
import { downloadSpeechModel, selectSpeechModel, speechKey, useSpeechModel, useSpeechModels } from "../components/local-dictation";
import { Button, ErrorText } from "../components/ui";

export function Settings() {
  const qc = useQueryClient();
  const catalog = useSpeechModels();
  const active = useSpeechModel(catalog.data);
  const download = useMutation({ mutationFn: downloadSpeechModel, onSuccess: (data) => qc.setQueryData(speechKey, data) });
  return (
    <div className="mx-auto max-w-2xl space-y-6 px-2 py-5 sm:py-8">
      <h1 className="text-xl font-semibold">Ajustes</h1>
      <section aria-labelledby="dictation-title" className="space-y-3">
        <h2 id="dictation-title" className="font-semibold">Dictado local</h2>
        <p className="text-sm text-slate-600 dark:text-muted">El audio se procesa en este nodo. Descarga un modelo y elige cuál usar en el cuadro de mensaje.</p>
        <DictationShortcutSetting />
        {catalog.data && !catalog.data.runtime && <p role="status" className="text-sm text-amber-700 dark:text-amber-400">NeMo-Speech no está disponible en este nodo.</p>}
        {catalog.isPending && <p className="text-sm text-slate-500 dark:text-muted">Cargando modelos…</p>}
        <ErrorText error={catalog.error ?? download.error} />
        <ul aria-label="Modelos de dictado" className="divide-y divide-slate-200 rounded-xl border border-slate-200 bg-white dark:divide-border dark:border-border dark:bg-surface">
          {catalog.data?.models.map((model) => (
            <li key={model.id} className="flex flex-wrap items-center justify-between gap-3 px-4 py-3">
              <div>
                <h3 className="text-sm font-medium">{model.name}</h3>
                <p className="text-xs text-slate-500 dark:text-muted">
                  {model.streaming ? "Vista previa en directo" : "Transcribe al terminar"} · {Math.round(model.bytes / 2 ** 20)} MiB
                </p>
                {model.error && <p role="alert" className="mt-1 text-xs text-red-600 dark:text-red-400">{model.error}</p>}
              </div>
              {model.status === "installed" ? (
                active?.id === model.id ? <span className="inline-flex items-center gap-1.5 text-sm"><Check weight="thin" className="size-4" />En uso</span> :
                  <Button type="button" variant="ghost" disabled={!catalog.data?.runtime} onClick={() => selectSpeechModel(model.id)}>Usar modelo</Button>
              ) : (
                <Button type="button" variant="ghost" disabled={model.status === "downloading" || (download.isPending && download.variables === model.id)}
                  onClick={() => download.mutate(model.id)}>
                  <DownloadSimple weight="thin" className="mr-1.5 size-4" />
                  {model.status === "downloading" ? "Descargando…" : model.status === "failed" ? "Reintentar" : "Descargar"}
                </Button>
              )}
            </li>
          ))}
        </ul>
      </section>
    </div>
  );
}

function DictationShortcutSetting() {
  const chord = useDictationShortcutSetting();
  const [editing, setEditing] = useState(false);
  return (
    <div className="flex flex-wrap items-center justify-between gap-3 rounded-lg border border-slate-200 px-4 py-3 dark:border-border">
      <div className="space-y-1">
        <p className="text-sm font-medium">Iniciar o detener el dictado</p>
        <p className="text-xs text-slate-500 dark:text-muted">{editing ? "Pulsa Ctrl, Alt o ⌘ con una letra o un número. Esc cancela." : "Funciona en el editor activo o sobre el texto seleccionado en un hilo."}</p>
      </div>
      <div className="flex items-center gap-2">
        <Button type="button" variant="ghost" aria-pressed={editing} data-dictation-shortcut-capture
          onClick={(event) => { event.currentTarget.focus(); setEditing(!editing); }}
          onBlur={() => setEditing(false)}
          onKeyDown={(event) => {
            if (!editing || event.nativeEvent.isComposing || event.repeat) return;
            if (event.key === "Escape") {
              event.preventDefault();
              event.stopPropagation();
              setEditing(false);
              return;
            }
            const next = chordFromEvent(event.nativeEvent);
            if (!next || event.getModifierState("AltGraph")) return;
            event.preventDefault();
            event.stopPropagation();
            saveDictationShortcut(next);
            setEditing(false);
          }}>
          {editing ? "Pulsa el atajo…" : formatChord(chord)}
        </Button>
        {formatChord(chord) !== "Ctrl+D" && <Button type="button" variant="bare" onClick={() => saveDictationShortcut()}>Restaurar Ctrl+D</Button>}
      </div>
    </div>
  );
}
