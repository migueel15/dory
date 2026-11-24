import { useState } from "react";
import { Button } from "../../components/ui";
import { chordFromEvent, formatChord } from "../composer/dictation-shortcut";
import { available, chordIn, commands, conflictIn, reserved, type CommandId } from "../../components/shortcuts";
import { emptyInput, ValueDiff } from "./input-settings";
import { useNodeSettings, type SectionProps } from "./settings-node";
import type { InputChord } from "../../api/client";

// Shortcuts are a setting of each node; with "Todos los nodos" a change is
// written to each node on top of its own list.
export function ShortcutsSettings({ node }: SectionProps) {
  const settings = useNodeSettings(node);
  const input = settings.get("input") ?? emptyInput;
  const get = (id: CommandId) => chordIn(input.shortcuts, id);
  const conflict = (id: CommandId, value: InputChord) => conflictIn(input.shortcuts, id, value);
  const save = (id: CommandId, value?: InputChord) => {
    // undefined drops the key when serialized, so each node loses only this command.
    settings.setPath(["input", "shortcuts", id], value);
  };
  const [editing, setEditing] = useState<CommandId | null>(null);
  const [error, setError] = useState("");
  return <section className="space-y-4" aria-labelledby="shortcuts-title">
    <h2 id="shortcuts-title" className="text-lg font-semibold">Atajos</h2>
    <p className="text-sm text-slate-600 dark:text-muted">Funcionan con Dory enfocado. El navegador o el sistema pueden reservar algunas combinaciones, como Ctrl+N. Algunas combinaciones con Mayús también pueden pertenecer al navegador. Puedes reasignarlas.</p>
    <Button type="button" variant="ghost" disabled={!settings.loaded || settings.pending} onClick={() => { settings.setPath(["input", "shortcuts"], {}); setError(""); }}>Restaurar todos</Button>
    {settings.error && <p role="alert" className="text-sm text-red-600 dark:text-red-400">{String(settings.error)}</p>}
    {error && <p role="alert" className="text-sm text-red-600 dark:text-red-400">{error}</p>}
    <ul className="divide-y divide-slate-200 rounded-xl border border-slate-200 dark:divide-border dark:border-border">
      {commands.map((command) => <li key={command.id} className="flex flex-wrap items-center gap-2 px-4 py-3">
        <div className="min-w-0 flex-1"><p className="text-sm font-medium">{command.label}</p><p className="text-xs text-slate-500 dark:text-muted">{conflict(command.id, get(command.id)) ? `Conflicto con ${conflict(command.id, get(command.id))!.label}` : command.id === "dictation" ? "Según el editor y la selección" : available(command.id) ? "Disponible ahora" : "Disponible según el contexto"}</p></div>
        <ValueDiff items={settings.values(["input", "shortcuts"]).map((v) => ({ label: v.label, text: formatChord(chordIn(v.value, command.id)) }))} />
        <Button type="button" variant="ghost" disabled={!settings.loaded || settings.pending} data-dictation-shortcut-capture aria-pressed={editing === command.id}
          onClick={(e) => { e.currentTarget.focus(); setEditing(command.id); setError(""); }} onBlur={() => setEditing(null)}
          onKeyDown={(e) => {
            if (editing !== command.id || e.nativeEvent.isComposing || e.repeat) return;
            e.preventDefault(); e.stopPropagation();
            if (e.key === "Escape") { setEditing(null); return; }
            const next = chordFromEvent(e.nativeEvent);
            if (!next || e.getModifierState("AltGraph")) { setError("Pulsa Ctrl, Alt o ⌘ con una letra o un número."); return; }
            if (reserved(next)) { setError("Esta combinación está reservada para edición o zoom."); return; }
            const other = conflict(command.id, next);
            if (other) { setError(`Ya está asignada a ${other.label}.`); return; }
            save(command.id, next); setEditing(null); setError("");
          }}>{editing === command.id ? "Pulsa el atajo…" : formatChord(get(command.id))}</Button>
        <Button type="button" variant="bare" onClick={() => {
          const other = conflict(command.id, command.default);
          if (other) { setError(`El valor predeterminado ya está asignado a ${other.label}.`); return; }
          save(command.id); setError("");
        }}>Restaurar</Button>
      </li>)}
    </ul>
  </section>;
}
