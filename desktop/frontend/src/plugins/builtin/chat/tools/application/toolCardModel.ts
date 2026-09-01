import type { Translate } from "@/lib/i18n";
import type { ToolCall } from "@/plugins/sdk/types/agentSessionView";
import type { ActivityShell } from "@/lib/activityShell";
import {
  toolActivityShell,
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
  shell: ActivityShell;
  tone: "neutral" | "warning" | "negative";
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
    shell: toolActivityShell(tool),
    // Always neutral: lifecycle is carried by inline text/dot metadata, and colouring the
    // identity glyph turns errors and refusals back into status cards.
    tone: "neutral",
  };
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
