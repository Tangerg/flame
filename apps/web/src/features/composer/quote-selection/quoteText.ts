import type { QuoteSource } from "@flame/ui";
import { formatFileRef } from "@/features/agent/public/fileRefs";

export function quoteText(
  source: QuoteSource,
  text: string,
  lines: readonly [number, number] | null = null,
): string {
  const body = text.replace(/\s+$/, "");
  if (source.kind === "message") {
    const quote = body
      .split("\n")
      .map((line) => (line ? `> ${line}` : ">"))
      .join("\n");
    return `${quote}\n\n`;
  }
  let fence = "```";
  for (const match of body.matchAll(/`+/g)) {
    if (match[0].length >= fence.length) fence = match[0] + "`";
  }
  const block = [fence, body, fence].join("\n");
  if (source.kind === "tool-output") return `${block}\n\n`;
  const where = lines
    ? formatFileRef({ path: source.path, line: lines[0], column: 0 }, lines[1])
    : source.path;
  return `\`${where}\`\n${block}\n\n`;
}
