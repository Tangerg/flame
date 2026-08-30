import type { Message } from "@/plugins/sdk/types/agentSessionView";

/**
 * Only the prose-bearing kinds contribute. A UI-only block's `text` is a card LABEL, not
 * prose, so it must never leak into copied or exported plaintext.
 */
export function flattenText(blocks: Message["blocks"]): string {
  return blocks
    .map((b) => (b.kind === "text" || b.kind === "reasoning" ? b.text : ""))
    .filter(Boolean)
    .join("\n\n");
}

/**
 * Keeps the original markup, so the consumer sees the same headings, fences and lists they
 * were rendered from. UI-only kinds are dropped.
 */
export function flattenMarkdown(blocks: Message["blocks"]): string {
  const out: string[] = [];
  for (const b of blocks) {
    if (b.kind === "text" && b.text) {
      out.push(b.text);
    } else if (b.kind === "reasoning" && b.text) {
      const quoted = b.text
        .split("\n")
        .map((line) => `> *${line}*`)
        .join("\n");
      out.push(quoted);
    }
  }
  return out.join("\n\n");
}
