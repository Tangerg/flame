import type { Translate } from "@/lib/i18n";
import type { ToolCall } from "@/plugins/sdk/types/agentSessionView";
import {
  summarizeActivity,
  toolGroupNeedsAttention,
} from "@/plugins/builtin/agent/public/messagePresentation";

export type ToolGroupPinnedState = boolean | null;

export interface ToolGroupModel {
  summary: string;
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
    count: tools.length,
    needsAttention,
    expanded,
    nextPinned: !expanded,
  };
}
