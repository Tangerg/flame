import { MESSAGE_CONTENT_SELECTOR } from "@/features/chat/public/message/rendering";
function escapeRegExp(value: string): string {
  return value.replace(/[.*+?^${}()|[\]\\]/g, "\\$&");
}

// Formatting splits text nodes without splitting a phrase; block layout still separates flows.
function continuesInlineFlow(element: Element): boolean {
  const display = getComputedStyle(element).display;
  return display === "contents" || display.startsWith("inline");
}

function collectTextFlows(messageRoot: HTMLElement): Text[][] {
  const flows: Text[][] = [];
  let flow: Text[] = [];

  const endFlow = () => {
    if (flow.length > 0) flows.push(flow);
    flow = [];
  };

  const visit = (node: Node) => {
    if (node.nodeType === Node.TEXT_NODE) {
      flow.push(node as Text);
      return;
    }
    if (node.nodeType !== Node.ELEMENT_NODE) return;

    const element = node as Element;
    if (element.nodeName === "BR") {
      endFlow();
      return;
    }

    const inline = element !== messageRoot && continuesInlineFlow(element);
    if (!inline) endFlow();
    for (const child of element.childNodes) visit(child);
    if (!inline) endFlow();
  };

  visit(messageRoot);
  return flows;
}

function findFlowRanges(flow: Text[], pattern: RegExp, ranges: Range[]): void {
  let text = "";
  const segments = flow.map((node) => {
    const start = text.length;
    text += node.data;
    return { node, start, end: text.length };
  });

  for (const match of text.matchAll(pattern)) {
    const start = match.index;
    const end = start + match[0].length;
    // Every non-empty match lies within this flow, so both character offsets have a segment.
    const first = segments.find((segment) => segment.end > start)!;
    const last = segments.find((segment) => segment.end >= end)!;

    const range = document.createRange();
    range.setStart(first.node, start - first.start);
    range.setEnd(last.node, end - last.start);
    ranges.push(range);
  }
}

export function findMessageRanges(query: string, root: ParentNode = document): Range[] {
  if (!query) return [];

  const pattern = new RegExp(escapeRegExp(query), "gi");
  const ranges: Range[] = [];
  const messageRoots = root.querySelectorAll<HTMLElement>(MESSAGE_CONTENT_SELECTOR);

  for (const messageRoot of messageRoots) {
    for (const flow of collectTextFlows(messageRoot)) {
      findFlowRanges(flow, pattern, ranges);
    }
  }

  return ranges;
}
