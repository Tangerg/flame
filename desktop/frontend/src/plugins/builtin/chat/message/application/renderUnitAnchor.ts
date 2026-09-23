import type { MessageRenderUnit } from "@/plugins/builtin/agent/public/messagePresentation";

export const BLOCK_ANCHOR_ATTR = "data-block-anchor";

export function renderUnitAnchor(messageId: string, unit: MessageRenderUnit): string {
  if (unit.kind === "wave") {
    const first = unit.units[0];
    return first ? `${messageId}:w:${renderUnitAnchor(messageId, first)}` : `${messageId}:w:0`;
  }
  if (unit.kind === "toolGroup") return `${messageId}:g:${unit.tools[0]?.id ?? "0"}`;
  const { block, index } = unit;
  if (block.kind === "tool") return `${messageId}:t:${block.toolCallId}`;
  if ((block.kind === "approval" || block.kind === "question") && block.itemId) {
    return `${messageId}:i:${block.itemId}`;
  }
  return `${messageId}:b:${index}`;
}
