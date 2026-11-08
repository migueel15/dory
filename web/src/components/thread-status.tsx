import { ChatCircleDots, CheckCircle, Circle, CircleNotch, Clock, Prohibit, Question, StopCircle, Warning, XCircle, type Icon as PhosphorIcon, type IconWeight } from "@phosphor-icons/react";

type Props = { status: string; attention?: boolean };
type Look = { label: string; color: string; Glyph: PhosphorIcon; weight?: IconWeight; spin?: boolean };

const looks: [string[], Look][] = [
  [["failed", "error"], { label: "Error", color: "text-red-600 dark:text-red-400", Glyph: XCircle, weight: "fill" }],
  [["running", "working", "starting", "preparing"], { label: "Trabajando", color: "text-blue-600 dark:text-blue-400", Glyph: CircleNotch, spin: true }],
  [["waiting"], { label: "Esperando respuesta", color: "text-amber-600 dark:text-amber-400", Glyph: ChatCircleDots, weight: "fill" }],
  [["queued", "pending"], { label: "En cola", color: "text-slate-500 dark:text-slate-400", Glyph: Clock }],
  [["completed", "complete", "done"], { label: "Completado", color: "text-green-700 dark:text-green-400", Glyph: CheckCircle, weight: "fill" }],
  [["interrupted"], { label: "Interrumpido", color: "text-orange-600 dark:text-orange-400", Glyph: StopCircle, weight: "fill" }],
  [["cancelled", "canceled"], { label: "Cancelado", color: "text-slate-500 dark:text-slate-400", Glyph: Prohibit }],
  [["idle", "settled", "ready"], { label: "En reposo", color: "text-slate-400 dark:text-slate-500", Glyph: Circle }],
];

/** Status as an icon only; the name shows as a tooltip and to screen readers. */
export function ThreadStatus({ status, attention = false }: Props) {
  const s = status.toLowerCase();
  const look: Look = attention
    ? { label: "Te necesita", color: "text-amber-600 dark:text-amber-400", Glyph: Warning, weight: "fill" }
    : looks.find(([keys]) => keys.includes(s))?.[1] ?? { label: status || "Estado desconocido", color: "text-slate-400", Glyph: Question };
  const { label, color, Glyph, weight = "regular", spin } = look;
  return (
    <span role="img" title={label} aria-label={label} className={`inline-flex items-center ${color}`}>
      <Glyph aria-hidden="true" weight={weight} className={`size-4 ${spin ? "motion-safe:animate-spin" : ""}`} />
    </span>
  );
}
