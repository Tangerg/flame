export type QuoteSource =
  | { kind: "file"; path: string; lines: readonly [number, number] | null }
  | { kind: "message" }
  | { kind: "tool-output" };

const FENCE = "```";

export function quoteText(source: QuoteSource, text: string): string {
  const body = text.replace(/\s+$/, "");
  if (source.kind === "file") {
    const where = source.lines
      ? source.lines[0] === source.lines[1]
        ? `${source.path}:${source.lines[0]}`
        : `${source.path}:${source.lines[0]}-${source.lines[1]}`
      : source.path;
    return [`\`${where}\``, FENCE, body, FENCE].join("\n");
  }
  if (source.kind === "tool-output") return [FENCE, body, FENCE].join("\n");
  return body
    .split("\n")
    .map((line) => (line ? `> ${line}` : ">"))
    .join("\n");
}
