import { type UsageRange, type UsageRow } from "../api/client";

export type Tab = "cost" | "tokens" | "breakdown" | "limits";
export type Search = { tab?: Tab; range?: UsageRange; provider?: string; by?: "model" | "day"; envs?: string };
export const RANGES: [UsageRange, string][] = [["24h", "24 h"], ["7d", "7 días"], ["30d", "30 días"], ["90d", "90 días"]];
export const TABS: [Tab, string][] = [["cost", "Coste"], ["tokens", "Tokens"], ["breakdown", "Desglose"], ["limits", "Límites"]];
export const PROVIDER_NAMES: Record<string, string> = { claude: "Claude Code", codex: "Codex", opencode: "OpenCode", antigravity: "Antigravity" };
export const TYPE_COLORS = { input: "#6366f1", cache_read: "#14b8a6", cache_write: "#f59e0b", output: "#ec4899" };
export const TYPE_LABELS = { input: "Entrada", cache_read: "Lectura de caché", cache_write: "Escritura de caché", output: "Salida" };
export type Kind = keyof typeof TYPE_COLORS;
export const KINDS = Object.keys(TYPE_COLORS) as Kind[];

export const tokensOf = (r: UsageRow) => r.tokens.input + r.tokens.output + r.tokens.cache_write + r.tokens.cache_read;
export const costOf = (r: UsageRow) => r.cost.input + r.cost.output + r.cost.cache_write + r.cost.cache_read;

export const cx = (...c: (string | false | undefined)[]) => c.filter(Boolean).join(" ");
export const date = (iso: string) => new Date(iso).toLocaleString("es-ES", { dateStyle: "short", timeStyle: "short" });
