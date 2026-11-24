import { useQueryClient } from "@tanstack/react-query";
import { useSetTitleModel,useTitleModel } from "../../api/queries/settings";
import { ErrorText,Select } from "../../components/ui";
import { useProviders } from "../composer/launch-picker";

// IdeasSettings picks the provider and model that write the titles of the
// ideas saved in «Nueva idea». Without one, ideas keep their first sentence.
export function IdeasSettings() {
  const qc = useQueryClient();
  const providers = useProviders();
  const current = useTitleModel();
  const save = useSetTitleModel({ onSuccess: (data) => qc.setQueryData(["title-model"], data) });
  const value = current.data?.provider ? `${current.data.provider}\t${current.data.model}` : "";
  const options = [{ value: "", label: "Sin títulos automáticos" },
    ...(providers.data ?? []).flatMap((p) => p.models.map((m) => ({ value: `${p.id}\t${m.slug}`, label: `${p.name} · ${m.name}` })))];
  if (value && !options.some((o) => o.value === value)) options.push({ value, label: `${current.data!.provider} · ${current.data!.model} (no disponible)` });
  return (
    <section aria-labelledby="ideas-title" className="space-y-3">
      <h2 id="ideas-title" className="text-lg font-semibold">Títulos de las ideas</h2>
      <p className="text-sm text-slate-600 dark:text-muted">Al guardar una idea, este modelo propone en segundo plano un título breve. La descripción conserva siempre el texto completo.</p>
      <Select value={value} disabled={current.isPending || save.isPending}
        onChange={(v) => { const [provider = "", model = ""] = v.split("\t"); save.mutate({ provider, model }); }} options={options} />
      <ErrorText error={current.error ?? save.error} />
    </section>
  );
}
