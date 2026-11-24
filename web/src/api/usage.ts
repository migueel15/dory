import { get, tz } from "./http";
import type { UsageBreakdownRow, UsageReport, UsageTurn } from "./types.gen";

export type UsageRange = "24h" | "7d" | "30d" | "90d";

export const usageApi = {
  usageBreakdown: (range: UsageRange, node = "local") => get<UsageBreakdownRow[]>(`/nodes/${node}/usage/breakdown?${new URLSearchParams({ range, tz: tz() })}`),
  threadUsageTurns: (thread: string, range: UsageRange, node: string) => get<UsageTurn[]>(`/nodes/${node}/usage/threads/${encodeURIComponent(thread)}/turns?${new URLSearchParams({ range, tz: tz() })}`),
  usageHistory: (range: UsageRange, node = "local") => get<UsageReport>(`/nodes/${node}/usage/history?${new URLSearchParams({ range, tz: tz() })}`),
};
