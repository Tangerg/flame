// Derived entirely from tool calls the fold already holds; the RUNTIME measures
// `durationMillis` per call, so nothing is timed in the client.

import type { ToolCall } from "@/plugins/sdk/types/agentSessionView";

export interface ToolStat {
  /** Wire tool identity — the same name the icon and preview registries key on. */
  name: string;
  calls: number;
  failed: number;
  denied: number;
  /** Summed across calls that REPORTED one: a running tool has no duration yet, and
   *  counting it as zero drags the total down. */
  totalMs: number;
  slowestMs: number;
  durations: number[];
  /** The denominator behind the two above, and why a row can show "—" rather than a
   *  fabricated zero. */
  timed: number;
}

export interface ToolStatsSummary {
  rows: ToolStat[];
  calls: number;
  failed: number;
  denied: number;
  totalMs: number;
}

/**
 * Ordered by TOTAL TIME rather than call count: twenty greps that cost nothing
 * are not the reason a session took ten minutes, and the row a reader is looking
 * for is the one that spent the time. Ties fall back to call count so a set of
 * untimed tools still has a stable order.
 */
export function toolStats(calls: Record<string, ToolCall>): ToolStatsSummary {
  const byName = new Map<string, ToolStat>();

  for (const call of Object.values(calls)) {
    // A call in flight has no outcome and no duration, so counting it makes the totals
    // move backwards when it settles.
    if (call.status === "running" || call.status === "requires-action") continue;
    const row = byName.get(call.name) ?? {
      name: call.name,
      calls: 0,
      failed: 0,
      denied: 0,
      totalMs: 0,
      slowestMs: 0,
      durations: [],
      timed: 0,
    };
    row.calls += 1;
    // Counted APART: a denial is a person saying no, and lumping it with failures makes
    // an approval policy read as a broken tool.
    if (call.status === "err") row.failed += 1;
    if (call.status === "denied") row.denied += 1;
    if (call.durationMillis !== undefined) {
      row.timed += 1;
      row.totalMs += call.durationMillis;
      row.slowestMs = Math.max(row.slowestMs, call.durationMillis);
      row.durations.push(call.durationMillis);
    }
    byName.set(call.name, row);
  }

  const rows = [...byName.values()].sort(
    (a, b) => b.totalMs - a.totalMs || b.calls - a.calls || a.name.localeCompare(b.name),
  );

  return {
    rows,
    calls: rows.reduce((total, row) => total + row.calls, 0),
    failed: rows.reduce((total, row) => total + row.failed, 0),
    denied: rows.reduce((total, row) => total + row.denied, 0),
    totalMs: rows.reduce((total, row) => total + row.totalMs, 0),
  };
}

/** Zero when nothing was timed, so an all-untimed session draws no bars rather than a
 *  row of full ones. */
export function toolTimeShare(row: ToolStat, summary: ToolStatsSummary): number {
  return summary.totalMs > 0 ? row.totalMs / summary.totalMs : 0;
}
