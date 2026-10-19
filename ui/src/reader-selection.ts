import type { ClipboardEvent, KeyboardEvent } from "react";
import { isImeKey } from "./keyboard";

const EDITABLE =
  "input, textarea, select, [contenteditable]:not([contenteditable='false'])";

function isEditableTarget(reader: HTMLElement, target: EventTarget): boolean {
  const editable = (target as Element).closest(EDITABLE);
  return editable !== null && reader.contains(editable);
}

export function selectAllInReader(event: KeyboardEvent<HTMLElement>): void {
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
  if (isEditableTarget(reader, event.target)) return;
  const selection = reader.ownerDocument.getSelection();
  if (!selection) return;
  event.preventDefault();
  selection.selectAllChildren(reader);
}

// Select All spans the reader's DOM framing (labels, controls, virtual spacers),
// so a whole-reader copy writes the reader's presented text instead.
export function copyWholeReader(
  event: ClipboardEvent<HTMLElement>,
  text: string,
): void {
  if (
    event.defaultPrevented ||
    isEditableTarget(event.currentTarget, event.target) ||
    !isWholeReaderSelected(event.currentTarget)
  )
    return;
  event.preventDefault();
  event.clipboardData.setData("text/plain", text);
}

function isWholeReaderSelected(reader: Element): boolean {
  const selection = reader.ownerDocument.getSelection();
  if (selection?.rangeCount !== 1) return false;
  const range = selection.getRangeAt(0);
  return (
    !range.collapsed &&
    range.startContainer === reader &&
    range.startOffset === 0 &&
    range.endContainer === reader &&
    range.endOffset === reader.childNodes.length
  );
}
