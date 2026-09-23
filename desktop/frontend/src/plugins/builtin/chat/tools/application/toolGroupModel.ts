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
