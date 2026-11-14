import type { ClipboardEvent, KeyboardEvent } from "react";
import { isImeKey } from "./keyboard";

const EDITABLE =
  "input, textarea, select, [contenteditable]:not([contenteditable='false'])";

function isEditableTarget(reader: HTMLElement, target: EventTarget): boolean {
  const editable = (target as Element).closest(EDITABLE);
  return editable !== null && reader.contains(editable);
}

// The event owner is the focused reader; `content` is the presented source inside
// it, so framing such as a reader's path label stays outside the selected range.
export function selectAllInReader(
  event: KeyboardEvent<HTMLElement>,
  content: Element = event.currentTarget,
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
  if (!reader.contains(content) || isEditableTarget(reader, event.target))
    return;
  const selection = reader.ownerDocument.getSelection();
  if (!selection) return;
  event.preventDefault();
  selection.selectAllChildren(content);
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
  const line = element?.closest<HTMLElement>("[data-reader-line]") ?? null;
  return line && content.contains(line) ? line : null;
}

// These are live Range endpoints, not a second selection or a selected-row window.
export function readerSelectionLines(content: Element | null): number[] {
  if (!content) return [];
  const range = selectedRange(content);
  if (!range) return [];
  return [range.startContainer, range.endContainer].flatMap((node) => {
    const line = lineAt(content, node);
    return line ? [Number(line.dataset.readerLine)] : [];
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
        (node as Element).hasAttribute("data-reader-source")
      )
    )
      return null;
    if (offset === 0) return 0;
    if (offset === node.childNodes.length) return text.length;
    const next = node.childNodes[offset];
    if (!next || next.nodeType !== Node.ELEMENT_NODE) return null;
    const element = next as Element;
    const nextLine = element.matches("[data-reader-line]")
      ? element
      : element.querySelector("[data-reader-line]");
    return nextLine ? sourceOffset(content, text, nextLine, 0) : null;
  }
  const index = Number(line.dataset.readerLine);
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
  if (
    !whole &&
    !content.matches("[data-reader-source], [data-reader-line]") &&
    !content.querySelector("[data-reader-source], [data-reader-line]")
  )
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
