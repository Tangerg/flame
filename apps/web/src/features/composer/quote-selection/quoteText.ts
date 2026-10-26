export type QuoteSource =
  | { kind: "file"; path: string; lines: readonly [number, number] | null }
  | { kind: "message" }
  | { kind: "tool-output" };

export function quoteText(source: QuoteSource, text: string): string {
  const body = text.replace(/\s+$/, "");
  let quote: string;
  if (source.kind === "message") {
    quote = body
      .split("\n")
      .map((line) => (line ? `> ${line}` : ">"))
      .join("\n");
  } else {
    let fence = "```";
    for (const match of body.matchAll(/`+/g)) {
      if (match[0].length >= fence.length) fence = match[0] + "`";
    }
    quote = [fence, body, fence].join("\n");
    if (source.kind === "file") {
      const where = source.lines
        ? source.lines[0] === source.lines[1]
          ? `${source.path}:${source.lines[0]}`
          : `${source.path}:${source.lines[0]}-${source.lines[1]}`
        : source.path;
      quote = [`\`${where}\``, quote].join("\n");
    }
  }
  return `${quote}\n\n`;
}
