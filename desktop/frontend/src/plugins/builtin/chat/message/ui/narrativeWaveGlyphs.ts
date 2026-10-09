import type { IconName } from "@/ui/icons";
import type { MessageRenderUnit } from "@/plugins/builtin/agent/public/messagePresentation";
import type { ToolCall } from "@/plugins/sdk/types/agentSessionView";
import { toolGroupIconFor } from "@/plugins/builtin/agent/public/toolIcon";

export function waveGlyph(
  units: readonly MessageRenderUnit[],
  tools: readonly ToolCall[],
): IconName {
  if (tools.length > 0) return toolGroupIconFor(tools);
  return units.some((unit) => unit.kind === "block" && unit.block.kind === "reasoning")
    ? "brain"
    : "activity";
}
