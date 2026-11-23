import { type DelegatedRunNarrative } from "@/features/agent/public/conversation";
import { type TranscriptRow, type TurnFacts } from "@/features/agent/public/conversation";
export interface SubagentEntry {
  narrative: DelegatedRunNarrative;
  /** The delegating tool call; siblings share it. */
  delegationId: string;
  taskLabel?: string;
  facts: TurnFacts;
  ordinal: number;
  siblingCount: number;
}

export function subagentEntries(rows: readonly TranscriptRow[]): SubagentEntry[] {
  const entries = new Map<string, SubagentEntry>();
  for (const row of rows) {
    for (const [itemId, siblings] of Object.entries(row.facts.delegatedRuns)) {
      for (const [index, narrative] of siblings.entries()) {
        entries.set(narrative.run.id, {
          narrative,
          delegationId: itemId,
          taskLabel: row.facts.toolCalls[itemId]?.fn,
          facts: row.facts,
          ordinal: index + 1,
          siblingCount: siblings.length,
        });
      }
    }
  }
  return [...entries.values()];
}
