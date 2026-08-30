import type { IconName } from "@/ui/icons";
import type { MessageRenderUnit } from "@/plugins/builtin/agent/public/messagePresentation";
import type { ToolCall } from "@/plugins/builtin/agent/public/viewState";
import { toolIconFor, toolRoutingKey } from "@/plugins/builtin/chat/tools/public/toolIcon";

export function waveGlyph(
  units: readonly MessageRenderUnit[],
  toolCalls: Record<string, ToolCall>,
): IconName | undefined {
  for (const unit of units) {
    if (unit.kind === "toolGroup") {
      const first = unit.tools[0];
      if (first) return toolIconFor(toolRoutingKey(first));
      continue;
    }
    if (unit.kind !== "block") continue;
    if (unit.block.kind === "reasoning") return "sparkle";
    if (unit.block.kind === "tool") {
      const tool = toolCalls[unit.block.toolCallId];
      if (tool) return toolIconFor(toolRoutingKey(tool));
    }
  }

  return undefined;
}
