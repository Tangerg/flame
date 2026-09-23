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
  denied: boolean;
  intent: ToolIntent;
  detail?: ToolDetail;
  error?: string;
  diffStat?: { added: number; removed: number };
  metaItems: ToolMetaItem[];
}

function namesOneOfMany(detail: ToolDetail | undefined, tool: ToolCall): boolean {
  return detail?.kind === "path" && (tool.files ?? 1) > 1;
}

export function toolCardModel(t: Translate, tool: ToolCall): ToolCardModel {
  const intent = toolIntent(t, tool);
  const metaItems = toolMetaItems(t, tool);
  const diffStat = toolDiffStat(tool);
  return {
    running: tool.status === "running",
    denied: tool.status === "denied",
    intent,
    detail: namesOneOfMany(intent.detail, tool) ? undefined : intent.detail,
    ...(tool.status === "err" && tool.error ? { error: tool.error } : {}),
    diffStat,
    metaItems,
  };
}

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
