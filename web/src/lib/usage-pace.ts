// WINDOWS names each usage window the harnesses report and gives its length in minutes.
export const WINDOWS: Record<string, [string, number]> = {
  five_hour: ["5 horas", 300], seven_day: ["Semanal", 7 * 24 * 60], seven_day_opus: ["Semanal · Opus", 7 * 24 * 60],
  seven_day_sonnet: ["Semanal · Sonnet", 7 * 24 * 60], thirty_day: ["Mensual", 30 * 24 * 60], monthly: ["Mensual", 30 * 24 * 60],
};

// The bars show remaining allowance, so pace is remaining time as well.
export function expectedRemaining(w: { resets_at: string | null; window_minutes: number | null }, now = Date.now()): number | null {
  if (!w.resets_at || !w.window_minutes || !Number.isFinite(w.window_minutes) || w.window_minutes <= 0) return null;
  const reset = Date.parse(w.resets_at);
  if (!Number.isFinite(reset)) return null;
  return Math.max(0, Math.min(100, (reset - now) / (w.window_minutes * 60_000) * 100));
}

export function paceTitle(pace: number | null): string {
  return pace === null ? "" : ` · Ritmo esperado: ${Math.round(pace)} %`;
}
