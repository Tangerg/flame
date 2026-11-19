import { readingRow, type ReadingListHandle } from "@flame/ui";
import { MESSAGE_CONTENT_SELECTOR } from "@/features/chat/public/message/rendering";

export interface MessageMatch {
  readonly range: Range;
  readonly message: HTMLElement;
  readonly occurrence: number;
}
function escapeRegExp(value: string): string {
  return value.replace(/[.*+?^${}()|[\]\\]/g, "\\$&");
}

function isHidden(element: Element, style = getComputedStyle(element)): boolean {
  if (element.matches("[hidden], [inert], .sr-only")) return true;
  return (
    style.display === "none" || style.visibility === "hidden" || style.visibility === "collapse"
  );
}

function collapsesHorizontalSpace(style: CSSStyleDeclaration): boolean {
  const mode = style.getPropertyValue("white-space-collapse");
  if (mode) return mode === "collapse" || mode === "preserve-breaks";
  return (
    style.whiteSpace === "normal" ||
    style.whiteSpace === "nowrap" ||
    style.whiteSpace === "pre-line"
  );
}

function hasHiddenAncestor(element: Element): boolean {
  for (let ancestor = element.parentElement; ancestor; ancestor = ancestor.parentElement) {
    if (isHidden(ancestor)) return true;
  }
  return false;
}

type FlowText = { node: Text; collapseSpace: boolean };

function collectTextFlows(messageRoot: HTMLElement): FlowText[][] {
  const flows: FlowText[][] = [];
  let flow: FlowText[] = [];

  const endFlow = () => {
    if (flow.length > 0) flows.push(flow);
    flow = [];
  };

  const visit = (node: Node, collapseSpace = false) => {
    if (node.nodeType === Node.TEXT_NODE) {
      flow.push({ node: node as Text, collapseSpace });
      return;
    }
    if (node.nodeType !== Node.ELEMENT_NODE) return;

    const element = node as Element;
    const style = getComputedStyle(element);
    if (isHidden(element, style)) return;
    if (element.nodeName === "BR") {
      endFlow();
      return;
    }

    // Formatting splits text nodes without splitting a phrase.
    const inline =
      element !== messageRoot &&
      (style.display === "contents" || style.display.startsWith("inline"));
    const collapseChildren = collapsesHorizontalSpace(style) && !element.closest("code, pre");
    if (!inline) endFlow();
    for (const child of element.childNodes) visit(child, collapseChildren);
    if (!inline) endFlow();
  };

  visit(messageRoot);
  return flows;
}

function findFlowRanges(flow: FlowText[], pattern: RegExp, ranges: Range[]): void {
  const adjustments: { offset: number; removed: number }[] = [];
  let text = "";
  let sourceLength = 0;
  let previousSpaceEnd = -1;
  let removed = 0;
  const segments = flow.map(({ node, collapseSpace }) => {
    const start = sourceLength;
    sourceLength += node.length;
    if (collapseSpace) {
      text += node.data.replace(/[ \t]+/g, (spaces, offset: number) => {
        const continuesRun = start + offset === previousSpaceEnd;
        previousSpaceEnd = start + offset + spaces.length;
        const hidden = spaces.length - (continuesRun ? 0 : 1);
        if (hidden > 0) {
          removed += hidden;
          adjustments.push({ offset: previousSpaceEnd - removed, removed });
        }
        return continuesRun ? "" : " ";
      });
    } else {
      text += node.data;
    }
    return { node, start, end: sourceLength };
  });

  // Match boundaries advance in order, and the end of a projected space includes its full source run.
  let nextAdjustment = 0;
  const toSourceOffset = (offset: number): number => {
    while (nextAdjustment < adjustments.length && adjustments[nextAdjustment]!.offset <= offset) {
      nextAdjustment++;
    }
    return offset + (adjustments[nextAdjustment - 1]?.removed ?? 0);
  };

  for (const match of text.matchAll(pattern)) {
    const start = toSourceOffset(match.index);
    const end = toSourceOffset(match.index + match[0].length);
    // Every non-empty match lies within this flow, so both character offsets have a segment.
    const first = segments.find((segment) => segment.end > start)!;
    const last = segments.find((segment) => segment.end >= end)!;

    const range = document.createRange();
    range.setStart(first.node, start - first.start);
    range.setEnd(last.node, end - last.start);
    ranges.push(range);
  }
}

// Virtual rows are not kept in document order, so the reader's row order decides.
export function findMessageMatches(query: string, list: ReadingListHandle): MessageMatch[] {
  const root = list.getScrollableNode();
  if (!query || !root) return [];

  const pattern = new RegExp(escapeRegExp(query), "gi");
  const matches: MessageMatch[] = [];
  const messageRoots = [...root.querySelectorAll<HTMLElement>(MESSAGE_CONTENT_SELECTOR)].flatMap(
    (element) => {
      const row = readingRow(list, element);
      return row ? [{ element, index: row.index }] : [];
    },
  );
  messageRoots.sort((a, b) => a.index - b.index);

  for (const { element: messageRoot } of messageRoots) {
    if (hasHiddenAncestor(messageRoot)) continue;
    const ranges: Range[] = [];
    for (const flow of collectTextFlows(messageRoot)) {
      findFlowRanges(flow, pattern, ranges);
    }
    matches.push(
      ...ranges.map((range, occurrence) => ({ range, message: messageRoot, occurrence })),
    );
  }

  return matches;
}

export function observeMessageChanges(root: HTMLElement, changed: () => void): () => void {
  const observer = new MutationObserver((records) => {
    if (records.some(changesMessage)) changed();
  });
  observer.observe(root, {
    childList: true,
    characterData: true,
    subtree: true,
    attributes: true,
    attributeFilter: ["hidden", "inert", "class", "style"],
  });
  return () => observer.disconnect();
}

function changesMessage(record: MutationRecord): boolean {
  const target = record.target instanceof Element ? record.target : record.target.parentElement;
  if (target?.closest(MESSAGE_CONTENT_SELECTOR)) return true;
  if (record.type === "attributes" && target?.querySelector(MESSAGE_CONTENT_SELECTOR)) return true;
  return [...record.addedNodes, ...record.removedNodes].some(
    (node) =>
      node instanceof Element &&
      (node.matches(MESSAGE_CONTENT_SELECTOR) || node.querySelector(MESSAGE_CONTENT_SELECTOR)),
  );
}
