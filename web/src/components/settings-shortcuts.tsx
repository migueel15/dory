import { useState } from "react";
import { Button } from "./ui";
import { chordFromEvent, formatChord } from "./dictation-shortcut";
import { available, commands, conflict, reserved, resetShortcuts, saveShortcut, useShortcuts, type CommandId } from "./shortcuts";

export function ShortcutsSettings() {
  const get = useShortcuts();
  const [editing, setEditing] = useState<CommandId | null>(null);
  const [error, setError] = useState("");
  return <section className="space-y-4" aria-labelledby="shortcuts-title">
    <h2 id="shortcuts-title" className="text-lg font-semibold">Atajos</h2>
    <p className="text-sm text-slate-600 dark:text-muted">Funcionan con Dory enfocado. El navegador o el sistema pueden reservar algunas combinaciones, como Ctrl+N. Algunas combinaciones con Mayús también pueden pertenecer al navegador. Puedes reasignarlas.</p>
    <Button type="button" variant="ghost" onClick={() => { resetShortcuts(); setError(""); }}>Restaurar todos</Button>
    {error && <p role="alert" className="text-sm text-red-600 dark:text-red-400">{error}</p>}
    <ul className="divide-y divide-slate-200 rounded-xl border border-slate-200 dark:divide-border dark:border-border">
      {commands.map((command) => <li key={command.id} className="flex flex-wrap items-center gap-2 px-4 py-3">
        <div className="min-w-0 flex-1"><p className="text-sm font-medium">{command.label}</p><p className="text-xs text-slate-500 dark:text-muted">{command.id === "dictation" ? "Según el editor y la selección" : available(command.id) ? "Disponible ahora" : "Disponible según el contexto"}</p></div>
        <Button type="button" variant="ghost" data-dictation-shortcut-capture aria-pressed={editing === command.id}
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
            saveShortcut(command.id, next); setEditing(null); setError("");
          }}>{editing === command.id ? "Pulsa el atajo…" : formatChord(get(command.id))}</Button>
        <Button type="button" variant="bare" onClick={() => {
          const other = conflict(command.id, command.default);
          if (other) { setError(`El valor predeterminado ya está asignado a ${other.label}.`); return; }
          saveShortcut(command.id); setError("");
        }}>Restaurar</Button>
      </li>)}
    </ul>
  </section>;
}
