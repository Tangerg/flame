import { knownIconName } from "@/ui/icons";
import type { IconName } from "@/ui/icons";
import type { ToolResultShape } from "@/plugins/sdk";
import type { ToolCall } from "@/plugins/sdk/types/agentSessionView";
import { lookupExtensionByKey, TOOL_ICON } from "@/plugins/sdk";
import { defaultToolIconFor } from "../presentation/toolIconContributions";

export { defaultToolIconContributions } from "../presentation/toolIconContributions";

export function toolRoutingKey(tool: ToolCall): string {
  return tool.name;
}

export function toolShapeKey(shape: ToolResultShape): string {
  return `@shape/${shape}`;
}

export function toolIconFor(key: string): IconName {
  return (
    knownIconName(lookupExtensionByKey(TOOL_ICON, key)) ??
    knownIconName(defaultToolIconFor(key)) ??
    "tool"
  );
}

export function toolCallIconFor(tool: ToolCall): IconName {
  if (tool.status === "err") return "x";
  if (tool.status === "requires-action") return "alert";
  if (tool.status === "denied") return "stop";
  return toolIconFor(toolRoutingKey(tool));
}
