import type {
  DelegatedRunNarrative,
  TranscriptRow,
  TurnFacts,
} from "@/plugins/builtin/agent/public/conversation";

export interface SubagentEntry {
  narrative: DelegatedRunNarrative;
  facts: TurnFacts;
  ordinal: number;
  siblingCount: number;
}

export function subagentEntries(rows: readonly TranscriptRow[]): SubagentEntry[] {
  const entries = new Map<string, SubagentEntry>();
  for (const row of rows) {
    for (const siblings of Object.values(row.facts.delegatedRuns)) {
      for (const [index, narrative] of siblings.entries()) {
        entries.set(narrative.run.id, {
          narrative,
          facts: row.facts,
          ordinal: index + 1,
          siblingCount: siblings.length,
        });
      }
    }
  }
  return [...entries.values()];
}
