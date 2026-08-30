import type { MessageRenderUnit } from "@/plugins/builtin/agent/public/messagePresentation";

// An attribute rather than a registry of refs: "where is it on screen" is a layout question,
// and a registry would have to stay in sync with a list that re-renders per streamed token.
export const BLOCK_ANCHOR_ATTR = "data-block-anchor";

/**
 * Serves as both the React key and the DOM anchor. IDENTITY, not position, wherever the
 * block has one: HITL cards hold per-interrupt local state, and keying by index reuses the
 * instance when a different interrupt lands in the same slot. Only blocks with nothing
 * better fall back to the index.
 */
export function renderUnitAnchor(messageId: string, unit: MessageRenderUnit): string {
  // A folded wave BORROWS the identity of what it holds, so opening it and rendering its
  // members inline does not remount them.
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
