import type { ClipboardEvent, KeyboardEvent } from "react";
import { isImeKey } from "./keyboard";

const EDITABLE =
  "input, textarea, select, [contenteditable]:not([contenteditable='false'])";
const SOURCE = "data-reader-source";
const LINE = "data-reader-line";
const MARKED = `[${SOURCE}], [${LINE}]`;
const WEBKIT_LINE_SCROLL = 40;

// A reader marks its source root and every presented line, so a copied range
// maps back to the exact source text instead of the rendered DOM text.
export const readerSourceProps = { [SOURCE]: "" } as const;

export function markReaderSource(element: Element): void {
  element.setAttribute(SOURCE, "");
}

export function readerLineProps(index: number) {
  return { [LINE]: index };
}

export function markReaderLine(element: Element, index: number): void {
  element.setAttribute(LINE, String(index));
}

function lineIndex(line: Element): number {
  return Number(line.getAttribute(LINE));
}

function isEditableTarget(reader: HTMLElement, target: EventTarget): boolean {
  const editable = (target as Element).closest(EDITABLE);
  return editable !== null && reader.contains(editable);
}

// The event owner is the focused reader; `content` is the presented source inside
// it, so framing such as a reader's path label stays outside the selected range.
export function selectAllInReader(
  event: KeyboardEvent<HTMLElement>,
  content: Element | ShadowRoot = event.currentTarget,
): void {
  if (
    event.defaultPrevented ||
    isImeKey(event.nativeEvent) ||
    event.altKey ||
    event.shiftKey ||
    (!event.ctrlKey && !event.metaKey) ||
    event.key.toLowerCase() !== "a"
  )
    return;
  const reader = event.currentTarget;
  const root = content.getRootNode();
  const ownsContent =
    reader.contains(content) ||
    (root instanceof ShadowRoot && reader.contains(root.host));
  if (!ownsContent || isEditableTarget(reader, event.target)) return;
  const selection = reader.ownerDocument.getSelection();
  if (!selection) return;
  event.preventDefault();
  selection.selectAllChildren(content);
}

function editingHost(node: Node): HTMLElement | null {
  const element = node instanceof Element ? node : node.parentElement;
  if (
    !(element instanceof HTMLElement) ||
    !element.isConnected ||
    !element.isContentEditable
  )
    return null;
  let host = element;
  while (host.parentElement?.isContentEditable) host = host.parentElement;
  return host;
}

function nearestVerticalScrollport(viewport: HTMLElement): HTMLElement | null {
  for (
    let region: HTMLElement | null = viewport;
    region;
    region = region.parentElement
  ) {
    const { display, overflowY, overscrollBehaviorY } =
      getComputedStyle(region);
    if (display === "contents" || display === "inline") continue;
    const userScrollable = overflowY === "auto" || overflowY === "scroll";
    // An overflowing scrollport still owns the key at either edge.
    if (
      userScrollable &&
      region.clientHeight > 0 &&
      region.scrollHeight > region.clientHeight
    )
      return region;
    if (
      (userScrollable || overflowY === "hidden") &&
      (overscrollBehaviorY === "contain" || overscrollBehaviorY === "none")
    )
      return null;
  }
  return null;
}

function readerArrowAxis(
  event: KeyboardEvent<HTMLElement>,
): "top" | "left" | null {
  const { currentTarget: viewport, key } = event;
  const vertical = key === "ArrowUp" || key === "ArrowDown";
  if (
    (!vertical && key !== "ArrowLeft" && key !== "ArrowRight") ||
    event.defaultPrevented ||
    !event.cancelable ||
    event.target !== viewport ||
    event.altKey ||
    event.ctrlKey ||
    event.metaKey ||
    event.shiftKey ||
    isImeKey(event.nativeEvent) ||
    viewport.ownerDocument.activeElement !== viewport
  )
    return null;
  return vertical ? "top" : "left";
}

function scrollReaderByArrow(
  event: KeyboardEvent<HTMLElement>,
  scroller: HTMLElement | null,
  axis: "top" | "left",
): void {
  event.preventDefault();
  const delta =
    event.key === "ArrowUp" || event.key === "ArrowLeft"
      ? -WEBKIT_LINE_SCROLL
      : WEBKIT_LINE_SCROLL;
  // A relative scroll preserves the step while the reader measures content.
  scroller?.scrollBy({ [axis]: delta, behavior: "instant" });
}

export function scrollReaderWithLocalArrow(
  event: KeyboardEvent<HTMLElement>,
): void {
  const axis = readerArrowAxis(event);
  if (!axis) return;
  const viewport = event.currentTarget;
  if (viewport.ownerDocument.getSelection()?.isCollapsed === false) return;

  const { clientHeight, clientWidth, scrollHeight, scrollWidth } = viewport;
  const verticalOverflow = clientHeight > 0 && scrollHeight > clientHeight;
  const horizontalOverflow = clientWidth > 0 && scrollWidth > clientWidth;
  // A horizontal key can otherwise jump to a distant WebKit caret in a tall reader.
  if (verticalOverflow || (axis === "left" && horizontalOverflow)) {
    scrollReaderByArrow(event, viewport, axis);
  }
}

export function scrollReaderWithForeignSelection(
  event: KeyboardEvent<HTMLElement>,
  axes: "vertical" | "both",
  verticalScroll: "local" | "nearest" = "local",
): void {
  const axis = readerArrowAxis(event);
  if (!axis || (axis === "left" && axes === "vertical")) return;
  const viewport = event.currentTarget;
  const selection = viewport.ownerDocument.getSelection();
  if (selection?.rangeCount !== 1) return;
  const { startContainer, endContainer } = selection.getRangeAt(0);
  const host = editingHost(startContainer);
  if (
    !host ||
    viewport.contains(host) ||
    host.contains(viewport) ||
    editingHost(endContainer) !== host
  )
    return;

  // WebKit can send the default arrow action to the editor retaining the selection.
  const scroller =
    axis === "top" && verticalScroll === "nearest"
      ? nearestVerticalScrollport(viewport)
      : viewport;
  scrollReaderByArrow(event, scroller, axis);
}

function selectedRange(content: Element): Range | null {
  const selection = content.ownerDocument.getSelection();
  if (selection?.rangeCount !== 1) return null;
  const range = selection.getRangeAt(0);
  return content.contains(range.startContainer) &&
    content.contains(range.endContainer)
    ? range
    : null;
}

function lineAt(content: Element, node: Node): HTMLElement | null {
  const element =
    node.nodeType === Node.ELEMENT_NODE
      ? (node as Element)
      : node.parentElement;
  const line = element?.closest<HTMLElement>(`[${LINE}]`) ?? null;
  return line && content.contains(line) ? line : null;
}

// These are live Range endpoints, not a second selection or a selected-row window.
export function readerSelectionLines(content: Element | null): number[] {
  if (!content) return [];
  const range = selectedRange(content);
  if (!range) return [];
  return [range.startContainer, range.endContainer].flatMap((node) => {
    const line = lineAt(content, node);
    return line ? [lineIndex(line)] : [];
  });
}

function sourceOffset(
  content: Element,
  text: string,
  node: Node,
  offset: number,
): number | null {
  const line = lineAt(content, node);
  if (!line) {
    if (
      node !== content &&
      !(
        node.nodeType === Node.ELEMENT_NODE &&
        (node as Element).hasAttribute(SOURCE)
      )
    )
      return null;
    if (offset === 0) return 0;
    if (offset === node.childNodes.length) return text.length;
    const next = node.childNodes[offset];
    if (!next || next.nodeType !== Node.ELEMENT_NODE) return null;
    const element = next as Element;
    const nextLine = element.matches(`[${LINE}]`)
      ? element
      : element.querySelector(`[${LINE}]`);
    return nextLine ? sourceOffset(content, text, nextLine, 0) : null;
  }
  const index = lineIndex(line);
  const lines = text.split("\n");
  if (
    !Number.isInteger(index) ||
    index < 0 ||
    index >= lines.length ||
    line.textContent !== lines[index]
  ) {
    throw new Error("reader line does not match its presented source");
  }
  const prefix = content.ownerDocument.createRange();
  prefix.setStart(line, 0);
  prefix.setEnd(node, offset);
  const within = prefix.toString().length;
  let start = 0;
  for (let i = 0; i < index; i++) start += lines[i]!.length + 1;
  return start + within;
}

// Marked virtual lines contain exactly their presented text. DOM UTF-16 offsets
// therefore address the same source, including styled spans, Unicode and blank lines.
export function copyReaderSelection(
  event: ClipboardEvent<HTMLElement>,
  text: string,
  content: Element = event.currentTarget,
): void {
  const reader = event.currentTarget;
  if (
    event.defaultPrevented ||
    !reader.contains(content) ||
    isEditableTarget(reader, event.target)
  )
    return;
  const range = selectedRange(content);
  if (!range || range.collapsed) return;
  const whole =
    range.startContainer === content &&
    range.startOffset === 0 &&
    range.endContainer === content &&
    range.endOffset === content.childNodes.length;
  if (!whole && !content.matches(MARKED) && !content.querySelector(MARKED))
    return;
  event.preventDefault();
  const start = whole
    ? 0
    : sourceOffset(content, text, range.startContainer, range.startOffset);
  const end = whole
    ? text.length
    : sourceOffset(content, text, range.endContainer, range.endOffset);
  if (start === null || end === null || start > end) {
    throw new Error("reader selection does not map to its presented source");
  }
  event.clipboardData.setData("text/plain", text.slice(start, end));
}
