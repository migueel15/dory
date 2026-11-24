import { useMutation, useQueryClient } from "@tanstack/react-query";
import { Check, DownloadSimple } from "@phosphor-icons/react";
import { downloadSpeechModel, pickSpeechModel, speechKey, useSpeechModels } from "../../components/local-dictation";
import { emptyInput, ValueDiff } from "./input-settings";
import { useNodeSettings, type SectionProps } from "./settings-node";
import { Button, ErrorText } from "../../components/ui";

// DictationSettings is the Dictado section of the settings: which speech model
// each node uses. The models are downloaded on this node, whose list is shown.
export function DictationSettings({ node }: SectionProps) {
  const settings = useNodeSettings(node);
  const qc = useQueryClient();
  const catalog = useSpeechModels();
  const input = settings.get("input") ?? emptyInput;
  const active = pickSpeechModel(catalog.data, input.dictation.model);
  const nameOf = (id?: string) => pickSpeechModel(catalog.data, id)?.name ?? "automático";
  const download = useMutation({ mutationFn: downloadSpeechModel, onSuccess: (data) => qc.setQueryData(speechKey, data) });
  return (
    <div className="space-y-6">
      <section aria-labelledby="dictation-title" className="space-y-3">
        <div className="flex items-center gap-2"><h2 id="dictation-title" className="text-lg font-semibold">Dictado local</h2><ValueDiff items={settings.values(["input", "dictation", "model"]).map((v) => ({ label: v.label, text: nameOf(v.value) }))} /></div>
        <p className="text-sm text-slate-600 dark:text-muted">El audio se procesa en este nodo. Descarga un modelo y elige cuál usar en el cuadro de mensaje.</p>
        <p className="text-sm text-slate-500 dark:text-muted">Configura el atajo de dictado en Ajustes → Atajos.</p>
        {catalog.data && !catalog.data.runtime && <p role="status" className="text-sm text-amber-700 dark:text-amber-400">NeMo-Speech no está disponible en este nodo.</p>}
        {catalog.isPending && <p className="text-sm text-slate-500 dark:text-muted">Cargando modelos…</p>}
        <ErrorText error={catalog.error ?? download.error ?? settings.error} />
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
                  <Button type="button" variant="ghost" disabled={!catalog.data?.runtime || !settings.loaded || settings.pending} onClick={() => settings.setPath(["input", "dictation", "model"], model.id)}>Usar modelo</Button>
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
