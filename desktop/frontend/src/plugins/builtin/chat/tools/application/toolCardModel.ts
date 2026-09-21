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
  /** The failure, for a slot that can hold all of it. Never the row's `detail`. */
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
    // The SUBJECT, whatever the outcome. A failure used to take this slot, which cost the row
    // the one thing that says which call failed — and bought the error nothing, because the
    // slot is a single truncating line, so a message longer than the row was unreadable and
    // uncopyable. It travels beside the row now, where it can be read in full.
    //
    // Withheld when it would name ONE of several. A `path` detail is a file, and a call that
    // reports touching three of them put whichever one it happened to carry beside a "3 files"
    // count — an arbitrary pick, read as THE file. The count says how many and the disclosure
    // names them all. Only `path`: a pattern or a URL is not one of the files.
    detail: namesOneOfMany(intent.detail, tool) ? undefined : intent.detail,
    ...(tool.status === "err" && tool.error ? { error: tool.error } : {}),
    diffStat,
    metaItems,
  };
}

/**
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
