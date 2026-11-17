import { quoteText, type QuoteSource } from "./quoteText";
function elementFor(node: Node): Element | null {
  const element = node instanceof Element ? node : node.parentElement;
  if (element?.closest("[data-quote-source]")) return element;
  const root = node.getRootNode();
  return root instanceof ShadowRoot ? elementFor(root.host) : element;
}
function quotedLine(node: Node): number {
  const element = node instanceof Element ? node : node.parentElement;
  return Number(element?.closest<HTMLElement>("[data-quote-line]")?.dataset.quoteLine);
}
export function captureQuote(): string | null {
  const selection = document.getSelection();
  if (!selection || selection.isCollapsed) return null;
  const shadows = [...document.querySelectorAll("diffs-container")].flatMap((element) =>
    element.shadowRoot ? [element.shadowRoot] : [],
  );
  const range = selection.getComposedRanges({ shadowRoots: shadows })[0];
  if (!range) return null;
  const start = elementFor(range.startContainer);
  const end = elementFor(range.endContainer);
  const host = start?.closest<HTMLElement>("[data-quote-source]");
  if (
    !host ||
    !host.isConnected ||
    host !== end?.closest("[data-quote-source]") ||
    host.closest("[hidden],[inert],[aria-hidden='true']")
  )
    return null;
  const text = selection.toString();
  if (!text.trim()) return null;
  let source: QuoteSource;
  if (host.dataset.quoteSource === "file") {
    const a = quotedLine(range.startContainer);
    const b = quotedLine(range.endContainer);
    source = {
      kind: "file",
      path: host.dataset.quotePath ?? "",
      lines: a > 0 && b > 0 ? [Math.min(a, b), Math.max(a, b)] : null,
    };
  } else if (host.dataset.quoteSource === "message" || host.dataset.quoteSource === "tool-output")
    source = { kind: host.dataset.quoteSource };
  else return null;
  return quoteText(source, text);
}
