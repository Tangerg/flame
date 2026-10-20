import { knownIconName } from "@/ui/icons/index";
import { type IconName } from "@/ui/icons/index";
import { type ToolCall } from "@/features/agent/public/sessionView";
import { lookupExtensionByKey } from "@/extensions/selectors/extensions";
import { TOOL_ICON } from "@/features/chat/public/tools/types";
import { defaultToolIconFor } from "@/features/agent/presentation/toolIconContributions";
export { defaultToolIconContributions } from "@/features/agent/presentation/toolIconContributions";

function toolRoutingKey(tool: ToolCall): string {
  return tool.name;
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
  if (tool.status === "denied") return "stop";
  return toolIconFor(toolRoutingKey(tool));
}

export function toolGroupIconFor(tools: readonly ToolCall[]): IconName {
  const first = tools[0];
  if (!first) return "activity";
  const icon = toolIconFor(toolRoutingKey(first));
  return tools.every((tool) => toolIconFor(toolRoutingKey(tool)) === icon) ? icon : "activity";
}
