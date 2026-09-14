import type {
  DelegatedRunNarrative,
  TranscriptRow,
  TurnFacts,
} from "@/plugins/builtin/agent/public/conversation";

export interface SubagentEntry {
  narrative: DelegatedRunNarrative;
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
