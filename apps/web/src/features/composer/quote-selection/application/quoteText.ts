export type QuoteSource =
  | { kind: "file"; path: string; lines: readonly [number, number] | null }
  | { kind: "message" }
  | { kind: "tool-output" };

const FENCE = "```";

export function quoteText(source: QuoteSource, text: string): string {
  const body = text.replace(/\s+$/, "");
  let quote: string;
  if (source.kind === "file") {
    const where = source.lines
      ? source.lines[0] === source.lines[1]
        ? `${source.path}:${source.lines[0]}`
        : `${source.path}:${source.lines[0]}-${source.lines[1]}`
      : source.path;
    quote = [`\`${where}\``, FENCE, body, FENCE].join("\n");
  } else if (source.kind === "tool-output") {
    quote = [FENCE, body, FENCE].join("\n");
  } else {
    quote = body
      .split("\n")
      .map((line) => (line ? `> ${line}` : ">"))
      .join("\n");
  }
  return `${quote}\n\n`;
}
