import { useEffect, useState } from "react";
import { ErrorText } from "../../components/ui";
import { NodeDiff, SettingRow, useNodeDefault, useNodeSettings, type NodeSettingsView, type SectionProps } from "./settings-node";

type Worktrees = { location: string; branch_naming: string; remove_on_merge: boolean };
const empty: Worktrees = { location: "", branch_naming: "", remove_on_merge: false };

// TextSetting saves a text field of the worktrees group when it loses focus.
function TextSetting({ settings, field, title, help, placeholder, multiline }: { settings: NodeSettingsView; field: "location" | "branch_naming"; title: string; help: string; placeholder: string; multiline?: boolean }) {
  const current = { ...empty, ...settings.get("worktrees") }; // first node, only to show the text
  const [text, setText] = useState(current[field]);
  useEffect(() => setText(current[field]), [current[field]]); // eslint-disable-line react-hooks/exhaustive-deps
  const props = {
    className: "mt-1 w-full rounded border border-slate-300 bg-transparent px-2 py-1 text-sm dark:border-white/15", value: text, placeholder, "aria-label": title,
    disabled: !settings.loaded || settings.pending, onChange: (e: { target: { value: string } }) => setText(e.target.value),
    onBlur: () => text !== current[field] && settings.setPath(["worktrees", field], text.trim()),
  };
  return (
    <div className="flex items-start gap-3 text-sm">
      <span className="min-w-0 flex-1"><strong className="block font-medium">{title}</strong><span className="text-slate-600 dark:text-muted">{help}</span>{multiline ? <textarea rows={3} {...props} /> : <input type="text" {...props} />}</span>
      <NodeDiff settings={settings} field={["worktrees", field]} />
    </div>
  );
}

// WorktreesSettings is the "Worktrees" section: where new worktrees go, how their
// branches are named and when they are removed.
export function WorktreesSettings({ node }: SectionProps) {
  const settings = useNodeSettings(node);
  const location = useNodeDefault(node, (d) => d.worktrees, "~/.dory/worktrees");
  return (
    <section aria-labelledby="worktrees-title" className="space-y-4">
      <h2 id="worktrees-title" className="text-lg font-semibold">Worktrees</h2>
      <TextSetting settings={settings} field="location" title="Ubicación por defecto" placeholder={location}
        help="Ruta absoluta de la carpeta donde se crean los worktrees nuevos. Vacío usa la ruta que se muestra. Los worktrees existentes no se mueven." />
      <TextSetting settings={settings} field="branch_naming" title="Convención de nombres de rama" multiline placeholder="p. ej. feat/<número>-<descripción en kebab-case>"
        help="Instrucción libre. Un modelo pequeño (el modelo de títulos del nodo) genera el nombre de la rama del worktree nuevo a partir del hilo y de esta instrucción. Si falla o no hay modelo, se usa el nombre automático." />
      <SettingRow settings={settings} field="remove_worktree_on_archive" title="Eliminar el worktree al borrar o archivar el hilo" help="Se borra su worktree si está limpio y ningún otro hilo lo usa. Con cambios sin commitear se conserva y el hilo archivado lo avisa." />
      <SettingRow settings={settings} field={["worktrees", "remove_on_merge"]} title="Eliminar el worktree al mergear su rama"
        help="Cuando la pull request de la tarea del hilo se mergea, se borra su worktree si está limpio y ningún otro hilo activo lo usa."
        control={(value, set, disabled) => <input type="checkbox" className="mt-1" aria-label="Eliminar el worktree al mergear su rama" checked={!!value} disabled={disabled} onChange={(e) => set(e.target.checked)} />} />
      <ErrorText error={settings.error} />
    </section>
  );
}
