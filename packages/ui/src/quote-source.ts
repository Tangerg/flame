export type QuoteSource =
  | { readonly kind: "file"; readonly path: string }
  | { readonly kind: "message" }
  | { readonly kind: "tool-output" };

export type QuoteKind = QuoteSource["kind"];

const SOURCE = "data-quote-source";
const PATH = "data-quote-path";
const LINE = "data-quote-line";
const KINDS: ReadonlySet<string> = new Set<QuoteKind>([
  "file",
  "message",
  "tool-output",
]);

// Readers in the host and in installed components declare where quoted text came
// from; the quote command reads the same attributes back from the selection.
export function quoteSourceProps(source: QuoteSource): Record<string, string> {
  if (source.kind !== "file") return { [SOURCE]: source.kind };
  if (!source.path)
    throw new Error("quote source: a file reader requires its path");
  return { [SOURCE]: source.kind, [PATH]: source.path };
}

export function markQuoteLine(element: Element, line: number): void {
  element.setAttribute(LINE, String(line));
}

export interface QuotedReader {
  readonly host: HTMLElement;
  readonly source: QuoteSource;
}

// A composed selection can end inside a reader's shadow root; its host carries
// the declaration.
function declaringElement(node: Node): Element | null {
  const element = node instanceof Element ? node : node.parentElement;
  if (element?.closest(`[${SOURCE}]`)) return element;
  const root = node.getRootNode();
  return root instanceof ShadowRoot ? declaringElement(root.host) : element;
}

export function quotedReaderOf(node: Node): QuotedReader | null {
  const host = declaringElement(node)?.closest<HTMLElement>(`[${SOURCE}]`);
  if (!host) return null;
  const kind = host.getAttribute(SOURCE)!;
  if (!KINDS.has(kind))
    throw new Error(`quote source: unknown reader kind ${kind}`);
  if (kind !== "file")
    return { host, source: { kind: kind as Exclude<QuoteKind, "file"> } };
  const path = host.getAttribute(PATH);
  if (!path) throw new Error("quote source: a file reader requires its path");
  return { host, source: { kind, path } };
}

export function quotedLineOf(node: Node): number | null {
  const element = node instanceof Element ? node : node.parentElement;
  const line = element?.closest(`[${LINE}]`)?.getAttribute(LINE);
  return line ? Number(line) : null;
}
