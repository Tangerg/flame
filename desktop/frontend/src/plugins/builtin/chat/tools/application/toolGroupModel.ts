import type { Translate } from "@/lib/i18n";
import type { ToolCall } from "@/plugins/sdk/types/agentSessionView";
import {
  summarizeActivity,
  toolGroupNeedsAttention,
} from "@/plugins/builtin/agent/public/messagePresentation";

export type ToolGroupPinnedState = boolean | null;

export interface ToolGroupModel {
  summary: string;
  dominantTool: string;
  count: number;
  needsAttention: boolean;
  expanded: boolean;
  nextPinned: boolean;
}

/**
 * Auto-open is for the LIVE wave only: once the turn starts answering the group is the
 * account of how it got there, not the thing in flight. A pin still wins. A failed child
 * does not force it open either — the row carries a flagged edge visible while closed,
 * which is what makes collapsing safe.
 */
export function toolGroupModel(
  t: Translate,
  tools: readonly ToolCall[],
  pinned: ToolGroupPinnedState,
  superseded = false,
): ToolGroupModel {
  const needsAttention = toolGroupNeedsAttention(tools);
  const expanded = pinned ?? (needsAttention && !superseded);
  return {
    summary: summarizeActivity(t, tools),
    dominantTool: dominantTool(tools),
    count: tools.length,
    needsAttention,
    expanded,
    nextPinned: !expanded,
  };
}

/**
 * A tie goes to whichever came FIRST, the tool the group opened with, which is also what the
 * summary counts. An empty group is not something the renderer produces but the type allows
 * it, so it answers with nothing and the glyph falls back.
 */
function dominantTool(tools: readonly ToolCall[]): string {
  const counts = new Map<string, number>();
  for (const tool of tools) counts.set(tool.name, (counts.get(tool.name) ?? 0) + 1);
  let dominant: string | undefined;
  let best = 0;
  for (const [name, count] of counts) {
    if (count > best) {
      best = count;
      dominant = name;
    }
  }
  return dominant ?? "";
}
