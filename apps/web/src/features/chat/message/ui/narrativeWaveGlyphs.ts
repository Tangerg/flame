import { type IconName } from "@/ui/icons/index";
import { type MessageRenderUnit } from "@/features/agent/public/messagePresentation";
import { type ToolCall } from "@/features/agent/public/sessionView";
import { toolGroupIconFor } from "@/features/agent/public/toolIcon";
export function waveGlyph(
  units: readonly MessageRenderUnit[],
  tools: readonly ToolCall[],
): IconName {
  if (tools.length > 0) return toolGroupIconFor(tools);
  return units.some((unit) => unit.kind === "block" && unit.block.kind === "reasoning")
    ? "brain"
    : "activity";
}
