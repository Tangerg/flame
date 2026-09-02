import type { Translate } from "@/lib/i18n";
import type { ToolCall } from "@/plugins/sdk/types/agentSessionView";
import {
  toolDiffStat,
  toolIntent,
  toolMetaItems,
  type ToolDetail,
  type ToolIntent,
  type ToolMetaItem,
} from "@/plugins/builtin/agent/public/messagePresentation";
import type { ToolActionSpec, ToolViewOpenerSpec } from "@/plugins/sdk";

export interface ToolCardModel {
  running: boolean;
  isError: boolean;
  denied: boolean;
  intent: ToolIntent;
  detail?: ToolDetail;
  diffStat?: { added: number; removed: number };
  metaItems: ToolMetaItem[];
}

export function toolCardModel(t: Translate, tool: ToolCall): ToolCardModel {
  const isError = tool.status === "err";
  const intent = toolIntent(t, tool);
  const metaItems = toolMetaItems(t, tool);
  const diffStat = toolDiffStat(tool);
  return {
    running: tool.status === "running",
    isError,
    denied: tool.status === "denied",
    intent,
    // Always `text`: a failure is prose, never a path, so it must not be left-truncated.
    detail: isError && tool.error ? { kind: "text", value: tool.error } : intent.detail,
    diffStat,
    metaItems,
  };
}

/**
 * The one figure a COMPACT row has space for.
 *
 * A failure outranks a measurement: a non-zero exit says something went wrong and a duration
 * only says how long it took, so the row must not spend its single slot on the second and
 * drop the first. Otherwise the last item wins, which is the most specific one the fold
 * derived — counts before spans before totals.
 */
export function headlineToolMetaItem(items: readonly ToolMetaItem[]): ToolMetaItem | undefined {
  return items.find((item) => item.tone === "negative") ?? items[items.length - 1];
}

export function toolCardActions(
  tool: ToolCall,
  actions: readonly ToolActionSpec[],
): ToolActionSpec[] {
  return actions.filter((action) => !action.predicate || action.predicate(tool));
}

export function toolCardViewOpener(
  tool: ToolCall,
  openers: readonly ToolViewOpenerSpec[],
): ToolViewOpenerSpec | undefined {
  return openers.find((opener) => opener.predicate(tool));
}
