type Props = { status: string; attention?: boolean };

/** Compact status indicator; unknown states retain their original text. */
export function ThreadStatus({ status, attention = false }: Props) {
  const normalized = status.toLowerCase();
  let label: string;
  let color: string;
  let icon: string;
  let animated = false;
  let showLabel = false;

  if (["failed", "error"].includes(normalized)) {
    label = "Error";
    color = "text-red-600 dark:text-red-400";
    icon = "×";
  } else if (attention || normalized === "attention") {
    label = "Te necesita";
    color = "text-amber-700 dark:text-amber-400 font-semibold";
    icon = "⚠";
    showLabel = true;
  } else if (["running", "working"].includes(normalized)) {
    label = "Corriendo";
    color = "text-blue-600 dark:text-blue-400";
    icon = "●";
    animated = true;
  } else if (["completed", "complete", "done"].includes(normalized)) {
    label = "Completado";
    color = "text-green-700 dark:text-green-400";
    icon = "✓";
  } else if (["idle", "waiting", "pending"].includes(normalized)) {
    label = "En espera";
    color = "text-slate-500 dark:text-slate-400";
    icon = "◷";
  } else {
    label = status || "Estado desconocido";
    color = "text-slate-500 dark:text-slate-400";
    icon = "○";
    showLabel = true;
  }

  return (
    <span role="img" title={label} aria-label={label} className={`inline-flex items-center gap-1 ${color}`}>
      <span aria-hidden="true" className={animated ? "motion-safe:animate-pulse" : undefined}>{icon}</span>
      {showLabel && <span aria-hidden="true">{label}</span>}
    </span>
  );
}
