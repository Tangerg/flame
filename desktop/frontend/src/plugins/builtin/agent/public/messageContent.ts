import type { Message } from "@/plugins/sdk/types/agentSessionView";

export function flattenText(blocks: Message["blocks"]): string {
  return blocks
    .map((b) => (b.kind === "text" || b.kind === "reasoning" ? b.text : ""))
    .filter(Boolean)
    .join("\n\n");
}

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
