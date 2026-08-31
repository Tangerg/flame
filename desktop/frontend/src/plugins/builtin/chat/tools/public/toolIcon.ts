import { knownIconName } from "@/ui/icons";
import type { IconName } from "@/ui/icons";
import type { ToolCall } from "@/plugins/sdk/types/agentSessionView";
import { lookupExtensionByKey, TOOL_ICON } from "@/plugins/sdk";
import { defaultToolIconFor } from "../application/toolIconContributions";

export { defaultToolIconContributions } from "../application/toolIconContributions";

/** The icon/preview registry key for a tool = its wire `name` (§4.4). */
export function toolRoutingKey(tool: ToolCall): string {
  return tool.name;
}

export function toolIconFor(key: string): IconName {
  // A plugin contributes its glyph as a plain string, so an unknown one would draw nothing at
  // all. The tool's own default is the next answer, and the generic tool mark the last.
  return (
    knownIconName(lookupExtensionByKey(TOOL_ICON, key)) ??
    knownIconName(defaultToolIconFor(key)) ??
    "tool"
  );
}

/** Status outranks identity in the leading mark; otherwise each tool keeps its
 * own registered glyph. Shared by standalone calls and grouped members so the
 * same invocation cannot change shape when it joins a group. */
export function toolCallIconFor(tool: ToolCall): IconName {
  if (tool.status === "err") return "x";
  if (tool.status === "requires-action") return "alert";
  if (tool.status === "denied") return "stop";
  return toolIconFor(toolRoutingKey(tool));
}
