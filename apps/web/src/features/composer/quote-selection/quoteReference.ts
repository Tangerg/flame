import type { QuoteSource } from "@flame/ui";
import { formatFileRef } from "@/features/agent/public/fileRefs";
import { type ComposerReference } from "@/features/composer/domain/draft";

const LABEL_CHARACTERS = 60;

export function quoteReference(
  source: QuoteSource,
  text: string,
  lines: readonly [number, number] | null = null,
): ComposerReference {
  const body = text.replace(/\s+$/, "");
  if (source.kind === "message") {
    const quote = body
      .split("\n")
      .map((line) => (line ? `> ${line}` : ">"))
      .join("\n");
    return { label: excerpt(body), text: quote };
  }
  let fence = "```";
  for (const match of body.matchAll(/`+/g)) {
    if (match[0].length >= fence.length) fence = match[0] + "`";
  }
  const block = [fence, body, fence].join("\n");
  if (source.kind === "tool-output") return { label: excerpt(body), text: block };
  const where = lines
    ? formatFileRef({ path: source.path, line: lines[0], column: 0 }, lines[1])
    : source.path;
  return { label: where, text: `\`${where}\`\n${block}` };
}

function excerpt(text: string): string {
  const line = text
    .split("\n")
    .map((candidate) => candidate.trim())
    .find(Boolean)!;
  const characters = [...line];
  return characters.length > LABEL_CHARACTERS
    ? `${characters.slice(0, LABEL_CHARACTERS).join("")}…`
    : line;
}
