import { ErrorText } from "./ui";
import { SettingRow, useNodeSettings, type SectionProps } from "./settings-node";

// SourceControlSettings is the "Control de versiones" section: what the selected
// node(s) do with the branches their threads run on.
export function SourceControlSettings({ node }: SectionProps) {
  const settings = useNodeSettings(node);
  return (
    <section aria-labelledby="source-control-title" className="space-y-4">
      <h2 id="source-control-title" className="text-lg font-semibold">Control de versiones</h2>
      <SettingRow settings={settings} field={["source_control", "auto_pull"]} title="Actualizar la rama del hilo automáticamente"
        help="Al iniciar un hilo y cada pocos minutos, Dory trae los cambios remotos de la rama del hilo (o del worktree de esa rama). Solo avanza en fast-forward y solo si no hay cambios sin commitear ni un turno en marcha."
        control={(value, set, disabled) => <input type="checkbox" className="mt-1" aria-label="Actualizar la rama del hilo automáticamente" checked={!!value} disabled={disabled} onChange={(e) => set(e.target.checked)} />} />
      <ErrorText error={settings.error} />
    </section>
  );
}
